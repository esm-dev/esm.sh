package storage

import (
	"bytes"
	"encoding/xml"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ije/gox/sync"
)

// A S3-compatible storage.
type s3Storage struct {
	apiEndpoint     string
	region          string
	accessKeyID     string
	secretAccessKey string
	fsCache         *fsStorage
	fsCacheLock     sync.KeyedMutex
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
	Contents []struct {
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
	if name == "" {
		return nil, errors.New("name is required")
	}
	if s3.fsCache != nil && !strings.HasSuffix(name, ".mjs.map") {
		stat, err = s3.fsCache.Stat(name)
		if err == nil {
			return
		}
		// ignore error
	}
	req, _ := http.NewRequest("HEAD", s3.apiEndpoint+"/"+name, nil)
	s3.sign(req)
	resp, err := http.DefaultClient.Do(req)
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
	if name == "" {
		return nil, nil, errors.New("name is required")
	}
	if s3.fsCache != nil && !strings.HasSuffix(name, ".mjs.map") {
		content, stat, err = s3.fsCache.Get(name)
		if err == nil {
			return
		}
		// ignore error
	}
	req, _ := http.NewRequest("GET", s3.apiEndpoint+"/"+name, nil)
	s3.sign(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	if resp.StatusCode == 404 {
		defer resp.Body.Close()
		err = ErrNotFound
		return
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
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
	if s3.fsCache != nil && !strings.HasSuffix(name, ".mjs.map") {
		pr, pw := io.Pipe()
		go func() {
			unlock := s3.fsCacheLock.Lock(name)
			defer unlock()
			_, err := s3.fsCache.Stat(name)
			if err == nil {
				_, err = io.Copy(pw, resp.Body)
				pw.CloseWithError(err)
				return
			}
			err = s3.fsCache.Put(name, io.TeeReader(resp.Body, pw))
			pw.CloseWithError(err)
			resp.Body.Close()
		}()
		content = pr
	} else {
		content = resp.Body
	}
	return
}

func (s3 *s3Storage) Put(name string, content io.Reader) (err error) {
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
	if s3.fsCache != nil && !strings.HasSuffix(name, ".mjs.map") {
		pr, pw := io.Pipe()
		go func(content io.Reader) {
			unlock := s3.fsCacheLock.Lock(name)
			defer unlock()
			err := s3.fsCache.Put(name, io.TeeReader(content, pw))
			pw.CloseWithError(err)
		}(content)
		content = pr
	}
	req, _ := http.NewRequest("PUT", s3.apiEndpoint+"/"+name, content)
	s3.sign(req)
	req.ContentLength = contentLength
	resp, err := http.DefaultClient.Do(req)
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
	if s3.fsCache != nil && !strings.HasSuffix(name, ".mjs.map") {
		go s3.fsCache.Delete(name)
	}
	req, _ := http.NewRequest("DELETE", s3.apiEndpoint+"/"+name, nil)
	s3.sign(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errors.New("unexpected status code: " + resp.Status)
	}
	return nil
}

func (s3 *s3Storage) List(prefix string) (keys []string, err error) {
	query := url.Values{}
	query.Set("list-type", "2")
	if prefix != "" {
		query.Set("prefix", prefix)
	}
	req, _ := http.NewRequest("GET", s3.apiEndpoint+"?"+query.Encode(), nil)
	s3.sign(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, parseS3Error(resp)
	}
	var ret s3ListResult
	err = xml.NewDecoder(resp.Body).Decode(&ret)
	if err != nil {
		return
	}
	keys = make([]string, len(ret.Contents))
	for i, content := range ret.Contents {
		keys[i] = content.Key
	}
	return
}

func (s3 *s3Storage) DeleteAll(prefix string) (deletedKeys []string, err error) {
	if prefix == "" {
		return nil, errors.New("prefix is required")
	}
	if s3.fsCache != nil {
		go s3.fsCache.DeleteAll(prefix)
	}
	keysToDelete, err := s3.List(prefix)
	if err != nil {
		return
	}
	if len(keysToDelete) == 0 {
		return []string{}, nil
	}
	buf := new(bytes.Buffer)
	buf.WriteString("<Delete>")
	for _, key := range keysToDelete {
		buf.WriteString("<Object><Key>")
		buf.WriteString(html.EscapeString(key))
		buf.WriteString("</Key></Object>")
	}
	buf.WriteString("</Delete>")
	req, _ := http.NewRequest("POST", s3.apiEndpoint+"?delete", buf)
	s3.sign(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, parseS3Error(resp)
	}
	var ret s3DeleteResult
	err = xml.NewDecoder(resp.Body).Decode(&ret)
	if err != nil {
		return
	}
	deletedKeys = make([]string, len(ret.Deleted))
	for i, deleted := range ret.Deleted {
		deletedKeys[i] = deleted.Key
	}
	return
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
	canonicalRequest := strings.Join([]string{req.Method, escapePath(req.URL.Path), req.URL.Query().Encode(), strings.Join(canonicalHeaders, "\n") + "\n", strings.Join(signedHeaders, ";"), req.Header.Get("X-Amz-Content-Sha256")}, "\n")
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", datetime, scope, toHex(sha256Sum(canonicalRequest))}, "\n")
	signingKey := hmacSum(hmacSum(hmacSum(hmacSum([]byte("AWS4"+s3.secretAccessKey), date), s3.region), "s3"), "aws4_request")
	signature := hmacSum(signingKey, stringToSign)
	req.Header.Set("Authorization", strings.Join([]string{"AWS4-HMAC-SHA256 Credential=" + s3.accessKeyID + "/" + scope, "SignedHeaders=" + strings.Join(signedHeaders, ";"), "Signature=" + toHex(signature)}, ", "))
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
