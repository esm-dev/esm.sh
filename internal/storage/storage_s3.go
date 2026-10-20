package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// [warning] only the wait for response headers is bounded, a body stream may take longer than any fixed timeout.
var s3Transport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 64
	transport.ResponseHeaderTimeout = time.Minute
	return transport
}()

// A S3-compatible storage.
type s3Storage struct {
	apiEndpoint     string
	region          string
	accessKeyID     string
	secretAccessKey string
	fsCache         *fsStorage
	// [warning] bumped after every S3 delete, a cache write that started before it must be dropped
	fsCacheEpoch atomic.Uint64
}

// NewS3Storage creates a new S3-compatible storage.
func NewS3Storage(options *StorageOptions) (Storage, error) {
	if options.Endpoint == "" {
		return nil, errors.New("missing endpoint")
	}
	u, err := url.Parse(options.Endpoint)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("invalid endpoint scheme")
	}
	if options.AccessKeyID == "" {
		return nil, errors.New("missing accessKeyID")
	}
	if options.SecretAccessKey == "" {
		return nil, errors.New("missing secretAccessKey")
	}
	storage := &s3Storage{
		apiEndpoint:     strings.TrimSuffix(u.String(), "/"),
		region:          options.Region,
		accessKeyID:     options.AccessKeyID,
		secretAccessKey: options.SecretAccessKey,
	}
	if options.CacheDir != "" {
		fs, err := NewFSStorage(options.CacheDir)
		if err != nil {
			return nil, err
		}
		storage.fsCache = fs.(*fsStorage)
	}
	return storage, nil
}

type s3ListResult struct {
	IsTruncated           bool
	NextContinuationToken string
	Contents              []struct {
		Key string
	}
}

type s3DeleteResult struct {
	Deleted []struct {
		Key string
	}
	Error []struct {
		Key     string
		Code    string
		Message string
	}
}

// s3ObjectMeta implements the Stat interface.
type s3ObjectMeta struct {
	contentLength int64
	lastModified  time.Time
}

func (s *s3ObjectMeta) Size() int64 {
	return s.contentLength
}

func (s *s3ObjectMeta) ModTime() time.Time {
	return s.lastModified
}

type s3Error struct {
	Code    string
	Message string
}

func (e s3Error) Error() string {
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

func (s3 *s3Storage) Stat(name string) (stat Stat, err error) {
	return s3.StatContext(context.Background(), name)
}

func (s3 *s3Storage) StatContext(ctx context.Context, name string) (stat Stat, err error) {
	if err = ctx.Err(); err != nil {
		return
	}
	if name == "" {
		return nil, errors.New("name is required")
	}
	if s3.shouldUseFSCache(name) {
		stat, err = s3.fsCache.Stat(s3.fsCacheKey(name))
		if err == nil {
			return
		}
		// ignore error
	}
	req, err := http.NewRequestWithContext(ctx, "HEAD", s3.apiEndpoint+"/"+escapePath(name), nil)
	if err != nil {
		return
	}
	resp, err := s3.do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		err = ErrNotFound
		return
	}
	if resp.StatusCode >= 400 {
		err = errors.New("unexpected status code: " + resp.Status)
		return
	}
	contentLengthHeader := resp.Header.Get("Content-Length")
	if contentLengthHeader == "" {
		err = errors.New("missing content size header")
		return
	}
	size, err := strconv.ParseInt(contentLengthHeader, 10, 64)
	if err != nil {
		err = errors.New("invalid content size header")
		return
	}
	lastModifiedHeader := resp.Header.Get("Last-Modified")
	if lastModifiedHeader == "" {
		err = errors.New("missing last modified header")
		return
	}
	lastModified, err := time.Parse(time.RFC1123, lastModifiedHeader)
	if err != nil {
		err = errors.New("invalid last modified header")
		return
	}
	stat = &s3ObjectMeta{
		contentLength: size,
		lastModified:  lastModified,
	}
	return
}

func (s3 *s3Storage) Get(name string) (content io.ReadCloser, stat Stat, err error) {
	return s3.GetContext(context.Background(), name)
}

func (s3 *s3Storage) GetContext(ctx context.Context, name string) (content io.ReadCloser, stat Stat, err error) {
	if name == "" {
		return nil, nil, errors.New("name is required")
	}
	if s3.shouldUseFSCache(name) {
		content, stat, err = s3.fsCache.Get(s3.fsCacheKey(name))
		if err == nil {
			return
		}
		// ignore error
	}
	epoch := s3.fsCacheEpoch.Load()
	req, err := http.NewRequestWithContext(ctx, "GET", s3.apiEndpoint+"/"+escapePath(name), nil)
	if err != nil {
		return
	}
	resp, err := s3.do(req)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			resp.Body.Close()
		}
	}()
	if resp.StatusCode == 404 {
		err = ErrNotFound
		return
	}
	if resp.StatusCode >= 400 {
		return nil, nil, parseS3Error(resp)
	}
	contentLengthHeader := resp.Header.Get("Content-Length")
	if contentLengthHeader == "" {
		err = errors.New("missing content size header")
		return
	}
	size, err := strconv.ParseInt(contentLengthHeader, 10, 64)
	if err != nil {
		err = errors.New("invalid content size header")
		return
	}
	lastModifiedHeader := resp.Header.Get("Last-Modified")
	if lastModifiedHeader == "" {
		err = errors.New("missing last modified header")
		return
	}
	lastModified, err := time.Parse(time.RFC1123, lastModifiedHeader)
	if err != nil {
		err = errors.New("invalid last modified header")
		return
	}
	stat = &s3ObjectMeta{
		contentLength: size,
		lastModified:  lastModified,
	}
	if s3.shouldUseFSCache(name) {
		cacheKey := s3.fsCacheKey(name)
		pr, pw := io.Pipe()
		go func() {
			defer resp.Body.Close()
			err := s3.fsCache.Put(cacheKey, io.TeeReader(resp.Body, pw))
			if err != nil {
				// [warning] the tee already passed every byte it read, so stream the rest when the cache write fails.
				// A read error of the response body is sticky, so the copy ends with it too.
				_, err = io.Copy(pw, resp.Body)
			} else if s3.fsCacheEpoch.Load() != epoch {
				s3.fsCache.Delete(cacheKey)
			}
			pw.CloseWithError(err)
		}()
		content = pr
	} else {
		content = resp.Body
	}
	return
}

func (s3 *s3Storage) Put(name string, content io.Reader) (err error) {
	return s3.PutContext(context.Background(), name, content)
}

func (s3 *s3Storage) PutContext(ctx context.Context, name string, content io.Reader) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}
	if name == "" {
		return errors.New("name is required")
	}
	var contentLength int64
	if buf, ok := content.(*bytes.Buffer); ok {
		contentLength = int64(buf.Len())
	} else if seeker, ok := content.(io.Seeker); ok {
		var size int64
		size, err = seeker.Seek(0, io.SeekEnd)
		if err != nil {
			return
		}
		_, err = seeker.Seek(0, io.SeekStart)
		if err != nil {
			return
		}
		contentLength = size
	} else if reader, ok := content.(*teeReader); ok {
		if buf, ok := reader.r.(*bytes.Buffer); ok {
			contentLength = int64(buf.Len())
		} else if seeker, ok := reader.r.(io.Seeker); ok {
			var size int64
			size, err = seeker.Seek(0, io.SeekEnd)
			if err != nil {
				return
			}
			_, err = seeker.Seek(0, io.SeekStart)
			if err != nil {
				return
			}
			contentLength = size
		} else {
			return errors.New("missing content length")
		}
	} else {
		err = errors.New("missing content length")
		return
	}
	if s3.shouldUseFSCache(name) {
		cacheKey := s3.fsCacheKey(name)
		epoch := s3.fsCacheEpoch.Load()
		pr, pw := io.Pipe()
		done := make(chan struct{})
		defer func() {
			pw.CloseWithError(err)
			<-done
		}()
		go func() {
			defer close(done)
			if err := s3.fsCache.Put(cacheKey, pr); err != nil {
				// [warning] drain the pipe, a failed cache write must not abort the upload
				io.Copy(io.Discard, pr)
			} else if s3.fsCacheEpoch.Load() != epoch {
				s3.fsCache.Delete(cacheKey)
			}
		}()
		content = io.TeeReader(content, pw)
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", s3.apiEndpoint+"/"+escapePath(name), content)
	if err != nil {
		return
	}
	req.ContentLength = contentLength
	if contentLength == 0 {
		// [warning] a body of unknown length is sent chunked, which S3 rejects
		req.Body = http.NoBody
	}
	resp, err := s3.do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return parseS3Error(resp)
	}
	return nil
}

func (s3 *s3Storage) Delete(name string) (err error) {
	if name == "" {
		return errors.New("key is required")
	}
	req, err := http.NewRequest("DELETE", s3.apiEndpoint+"/"+escapePath(name), nil)
	if err != nil {
		return err
	}
	resp, err := s3.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errors.New("unexpected status code: " + resp.Status)
	}
	if s3.shouldUseFSCache(name) {
		s3.fsCacheEpoch.Add(1)
		s3.fsCache.Delete(s3.fsCacheKey(name))
	}
	return nil
}

func (s3 *s3Storage) List(prefix string) (keys []string, err error) {
	query := url.Values{}
	query.Set("list-type", "2")
	if prefix != "" {
		query.Set("prefix", prefix)
	}
	keys = []string{}
	for {
		req, err := http.NewRequest("GET", s3.apiEndpoint+"?"+query.Encode(), nil)
		if err != nil {
			return keys, err
		}
		resp, err := s3.do(req)
		if err != nil {
			return keys, err
		}
		var ret s3ListResult
		if resp.StatusCode >= 400 {
			err = parseS3Error(resp)
		} else {
			err = xml.NewDecoder(resp.Body).Decode(&ret)
		}
		resp.Body.Close()
		if err != nil {
			return keys, err
		}
		for _, content := range ret.Contents {
			keys = append(keys, content.Key)
		}
		if !ret.IsTruncated {
			return keys, nil
		}
		if ret.NextContinuationToken == "" || ret.NextContinuationToken == query.Get("continuation-token") {
			return keys, errors.New("invalid S3 continuation token")
		}
		query.Set("continuation-token", ret.NextContinuationToken)
	}
}

func (s3 *s3Storage) DeleteAll(prefix string) (deletedKeys []string, err error) {
	if prefix == "" {
		return nil, errors.New("prefix is required")
	}
	keysToDelete, err := s3.List(prefix)
	if err != nil {
		return
	}
	if s3.fsCache != nil {
		defer func() {
			s3.fsCacheEpoch.Add(1)
			s3.fsCache.DeleteAll(prefix)
		}()
	}
	if len(keysToDelete) == 0 {
		return []string{}, nil
	}
	for batch := range slices.Chunk(keysToDelete, 1000) {
		buf := new(bytes.Buffer)
		buf.WriteString("<Delete>")
		for _, key := range batch {
			buf.WriteString("<Object><Key>")
			buf.WriteString(html.EscapeString(key))
			buf.WriteString("</Key></Object>")
		}
		buf.WriteString("</Delete>")
		checksum := md5.Sum(buf.Bytes())
		req, err := http.NewRequest("POST", s3.apiEndpoint+"?delete", buf)
		if err != nil {
			return deletedKeys, err
		}
		req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(checksum[:]))
		resp, err := s3.do(req)
		if err != nil {
			return deletedKeys, err
		}
		var ret s3DeleteResult
		if resp.StatusCode >= 400 {
			err = parseS3Error(resp)
		} else {
			err = xml.NewDecoder(resp.Body).Decode(&ret)
		}
		resp.Body.Close()
		if err != nil {
			return deletedKeys, err
		}
		for _, deleted := range ret.Deleted {
			deletedKeys = append(deletedKeys, deleted.Key)
		}
		for _, failure := range ret.Error {
			err = errors.Join(err, fmt.Errorf("failed to delete %q: %w", failure.Key, s3Error{Code: failure.Code, Message: failure.Message}))
		}
		if err != nil {
			return deletedKeys, err
		}
	}
	return
}

func (s3 *s3Storage) do(req *http.Request) (*http.Response, error) {
	s3.sign(req)
	client := *http.DefaultClient
	// [warning] keep a custom transport of the default client (tests, proxies)
	if client.Transport == nil || client.Transport == http.DefaultTransport {
		client.Transport = s3Transport
	}
	return client.Do(req)
}

// Authenticating Requests (AWS Signature Version 4)
// https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-authenticating-requests.html
func (s3 *s3Storage) sign(req *http.Request) {
	now := time.Now().UTC()
	date := now.Format("20060102")
	datetime := now.Format("20060102T150405Z")
	scope := date + "/" + s3.region + "/s3/aws4_request"
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("X-Amz-Date", datetime)
	signedHeaders := []string{"host"}
	for key := range req.Header {
		switch key {
		case "Host", "Accept-Encoding", "Authorization", "User-Agent":
			// ignore
		default:
			signedHeaders = append(signedHeaders, strings.ToLower(key))
		}
	}
	sort.Strings(signedHeaders)
	canonicalHeaders := make([]string, len(signedHeaders))
	for i, key := range signedHeaders {
		if key == "host" {
			canonicalHeaders[i] = key + ":" + req.Host
		} else {
			canonicalHeaders[i] = key + ":" + strings.Join(req.Header.Values(key), ",")
		}
	}
	uri := req.URL.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	canonicalRequest := strings.Join([]string{req.Method, uri, req.URL.Query().Encode(), strings.Join(canonicalHeaders, "\n") + "\n", strings.Join(signedHeaders, ";"), req.Header.Get("X-Amz-Content-Sha256")}, "\n")
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", datetime, scope, toHex(sha256Sum(canonicalRequest))}, "\n")
	signingKey := hmacSum(hmacSum(hmacSum(hmacSum([]byte("AWS4"+s3.secretAccessKey), date), s3.region), "s3"), "aws4_request")
	signature := hmacSum(signingKey, stringToSign)
	req.Header.Set("Authorization", strings.Join([]string{"AWS4-HMAC-SHA256 Credential=" + s3.accessKeyID + "/" + scope, "SignedHeaders=" + strings.Join(signedHeaders, ";"), "Signature=" + toHex(signature)}, ", "))
}

func (s3 *s3Storage) shouldUseFSCache(name string) bool {
	if strings.HasPrefix(name, "npm-resolution-slots/") {
		return false
	}
	if s3.fsCache == nil {
		return false
	}
	if strings.HasSuffix(name, ".mjs.map") || strings.HasSuffix(name, ".d.ts") || strings.HasSuffix(name, ".d.mts") || strings.HasSuffix(name, ".d.cts") {
		return false
	}
	return true
}

func (s3 *s3Storage) fsCacheKey(name string) string {
	const metaPrefix = "meta/"
	if strings.HasPrefix(name, metaPrefix) {
		hash := strings.TrimPrefix(name, metaPrefix)
		if len(hash) == 64 && strings.IndexByte(hash, '/') == -1 {
			return metaPrefix + hash[:2] + "/" + hash[2:]
		}
	}
	return name
}

func parseS3Error(resp *http.Response) error {
	var s3Error s3Error
	if xml.NewDecoder(resp.Body).Decode(&s3Error) != nil || s3Error.Code == "" {
		if resp.StatusCode == 429 {
			s3Error.Code = "TooManyRequests"
		} else {
			s3Error.Code = "UnexpectedStatusCode"
		}
		s3Error.Message = http.StatusText(resp.StatusCode)
	}
	if s3Error.Code == "NoSuchKey" {
		return ErrNotFound
	}
	return s3Error
}

func TeeReader(r io.Reader, w io.Writer) io.Reader {
	return &teeReader{r, w}
}

type teeReader struct {
	r io.Reader
	w io.Writer
}

func (t *teeReader) Read(p []byte) (n int, err error) {
	n, err = t.r.Read(p)
	if n > 0 {
		if n, err := t.w.Write(p[:n]); err != nil {
			return n, err
		}
	}
	return
}
