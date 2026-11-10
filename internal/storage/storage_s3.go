package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/maphash"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	bolt "go.etcd.io/bbolt"
)

// [warning] a source map that nobody reads within the TTL is removed, it is never uploaded
const sourceMapTTL = 7 * 24 * time.Hour

var (
	metaCacheBucket  = []byte("meta")
	sourceMapsBucket = []byte("source-maps")
	missSeed         = maphash.MakeSeed()
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
	metaCache       *bolt.DB
	// [warning] bumped after every S3 delete, a cache write that started before it must be dropped
	cacheEpoch atomic.Uint64
	// keys that S3 does not have, by hash
	misses *lru.Cache[uint64, struct{}]
	// [warning] a put bumps the slot of its key, a miss recorded across the bump must be dropped
	puts      [1024]atomic.Uint32
	uploads   sync.Map
	lastSweep atomic.Int64
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
	storage.misses, _ = lru.New[uint64, struct{}](1 << 19)
	if options.CacheDir != "" {
		fs, err := NewFSStorage(options.CacheDir)
		if err != nil {
			return nil, err
		}
		storage.fsCache = fs.(*fsStorage)
	}
	if options.MetaCacheFile != "" {
		if err := os.MkdirAll(filepath.Dir(options.MetaCacheFile), 0755); err != nil {
			return nil, err
		}
		db, err := bolt.Open(options.MetaCacheFile, 0600, &bolt.Options{Timeout: time.Second})
		if err != nil {
			return nil, err
		}
		err = db.Update(func(tx *bolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists(metaCacheBucket)
			if err == nil {
				_, err = tx.CreateBucketIfNotExists(sourceMapsBucket)
			}
			return err
		})
		if err != nil {
			db.Close()
			return nil, err
		}
		storage.metaCache = db
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
	if s3.shouldUseMetaCache(name) {
		if _, stat = s3.getMetaCache(name); stat != nil {
			return
		}
	}
	if s3.shouldUseFSCache(name) || s3.isLazySourceMap(name) {
		stat, err = s3.fsCache.Stat(name)
		if err == nil {
			return
		}
		// ignore error
	}
	if s3.isMissing(name) {
		return nil, ErrNotFound
	}
	seq := s3.putSeq(name).Load()
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
		s3.addMiss(name, seq)
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
	if s3.shouldUseMetaCache(name) {
		var data []byte
		if data, stat = s3.getMetaCache(name); stat != nil {
			return io.NopCloser(bytes.NewReader(data)), stat, nil
		}
	}
	if s3.shouldUseFSCache(name) || s3.isLazySourceMap(name) {
		content, stat, err = s3.fsCache.Get(name)
		if err == nil {
			if s3.isLazySourceMap(name) {
				go s3.uploadSourceMap(name)
			}
			return
		}
		// ignore error
	}
	if s3.isMissing(name) {
		return nil, nil, ErrNotFound
	}
	seq := s3.putSeq(name).Load()
	epoch := s3.cacheEpoch.Load()
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
		s3.addMiss(name, seq)
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
	if s3.shouldUseMetaCache(name) {
		var data []byte
		if data, err = io.ReadAll(resp.Body); err != nil {
			return nil, nil, err
		}
		resp.Body.Close()
		s3.putMetaCache(name, data, lastModified, epoch)
		content = io.NopCloser(bytes.NewReader(data))
	} else if s3.shouldUseFSCache(name) {
		pr, pw := io.Pipe()
		go func() {
			defer resp.Body.Close()
			err := s3.fsCache.Put(name, io.TeeReader(resp.Body, pw))
			if err != nil {
				// [warning] the tee already passed every byte it read, so stream the rest when the cache write fails.
				// A read error of the response body is sticky, so the copy ends with it too.
				_, err = io.Copy(pw, resp.Body)
			} else if s3.cacheEpoch.Load() != epoch {
				s3.fsCache.Delete(name)
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
	epoch := s3.cacheEpoch.Load()
	var metaData []byte
	if s3.shouldUseMetaCache(name) {
		if metaData, err = io.ReadAll(content); err != nil {
			return
		}
		content = bytes.NewReader(metaData)
	} else if s3.isLocalKey(name) || s3.isLazySourceMap(name) {
		if err = s3.fsCache.Put(name, content); err != nil {
			return
		}
		if s3.isLazySourceMap(name) {
			now := time.Now()
			err = s3.metaCache.Batch(func(tx *bolt.Tx) error {
				return tx.Bucket(sourceMapsBucket).Put(append(binary.BigEndian.AppendUint64(nil, uint64(now.Unix())), name...), nil)
			})
			if last := s3.lastSweep.Load(); now.Unix()-last > 3600 && s3.lastSweep.CompareAndSwap(last, now.Unix()) {
				go s3.sweepSourceMaps(now.Add(-sourceMapTTL))
			}
		}
		s3.markPut(name)
		return
	} else if s3.shouldUseFSCache(name) {
		pr, pw := io.Pipe()
		done := make(chan struct{})
		defer func() {
			pw.CloseWithError(err)
			<-done
		}()
		go func() {
			defer close(done)
			if err := s3.fsCache.Put(name, pr); err != nil {
				// [warning] drain the pipe, a failed cache write must not abort the upload
				io.Copy(io.Discard, pr)
			} else if s3.cacheEpoch.Load() != epoch {
				s3.fsCache.Delete(name)
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
	s3.markPut(name)
	if s3.shouldUseMetaCache(name) {
		s3.putMetaCache(name, metaData, time.Now(), epoch)
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
	if s3.shouldUseMetaCache(name) {
		s3.cacheEpoch.Add(1)
		// [warning] report a failed cache delete, the deleted metadata would be served again
		return s3.metaCache.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(metaCacheBucket).Delete([]byte(name))
		})
	}
	if s3.shouldUseFSCache(name) || s3.isLazySourceMap(name) {
		s3.cacheEpoch.Add(1)
		s3.fsCache.Delete(name)
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
	defer func() {
		s3.cacheEpoch.Add(1)
		if s3.fsCache != nil {
			s3.fsCache.DeleteAll(prefix)
		}
		if s3.metaCache != nil {
			err = errors.Join(err, s3.metaCache.Update(func(tx *bolt.Tx) error {
				cursor, p := tx.Bucket(metaCacheBucket).Cursor(), []byte(prefix)
				for k, _ := cursor.Seek(p); k != nil && bytes.HasPrefix(k, p); k, _ = cursor.Next() {
					if err := cursor.Delete(); err != nil {
						return err
					}
				}
				return nil
			}))
		}
	}()
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
	return s3.fsCache != nil && !isMetaKey(name) && !strings.HasPrefix(name, "npm-resolution-slots/") && !strings.HasSuffix(name, ".mjs.map")
}

// isLocalKey reports whether the key is saved in the local cache only.
// [warning] the objects are rebuilt when the cache is lost, use it for cheap builds only
func (s3 *s3Storage) isLocalKey(name string) bool {
	return s3.fsCache != nil && (strings.HasPrefix(name, "types/") || strings.HasPrefix(name, "exports/"))
}

// isLazySourceMap reports whether the source map waits in the local cache for its first read before the upload.
func (s3 *s3Storage) isLazySourceMap(name string) bool {
	return s3.fsCache != nil && s3.metaCache != nil && strings.HasSuffix(name, ".mjs.map")
}

// uploadSourceMap moves a source map from the local cache to S3.
func (s3 *s3Storage) uploadSourceMap(name string) {
	if _, uploading := s3.uploads.LoadOrStore(name, nil); uploading {
		return
	}
	defer s3.uploads.Delete(name)
	file, stat, err := s3.fsCache.Get(name)
	if err != nil {
		return
	}
	defer file.Close()
	req, err := http.NewRequest("PUT", s3.apiEndpoint+"/"+escapePath(name), file)
	if err != nil {
		return
	}
	req.ContentLength = stat.Size()
	if stat.Size() == 0 {
		req.Body = http.NoBody
	}
	resp, err := s3.do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	// [warning] keep a file that was written again during the upload, it is not uploaded yet
	if cur, err := s3.fsCache.Stat(name); resp.StatusCode < 400 && err == nil && cur.ModTime().Equal(stat.ModTime()) {
		s3.fsCache.Delete(name)
	}
}

// sweepSourceMaps removes the source maps written before the cutoff that are still in the local cache.
func (s3 *s3Storage) sweepSourceMaps(cutoff time.Time) {
	for {
		var keys [][]byte
		s3.metaCache.View(func(tx *bolt.Tx) error {
			cursor := tx.Bucket(sourceMapsBucket).Cursor()
			for k, _ := cursor.First(); k != nil && len(keys) < 1000 && int64(binary.BigEndian.Uint64(k)) < cutoff.Unix(); k, _ = cursor.Next() {
				keys = append(keys, bytes.Clone(k))
			}
			return nil
		})
		// [warning] remove the files outside of the write transaction, it blocks the metadata cache
		for _, k := range keys {
			// a rebuild writes the file again, keep it for its own record
			if stat, err := s3.fsCache.Stat(string(k[8:])); err == nil && stat.ModTime().Before(cutoff) {
				s3.fsCache.Delete(string(k[8:]))
			}
		}
		err := s3.metaCache.Update(func(tx *bolt.Tx) error {
			for _, k := range keys {
				if err := tx.Bucket(sourceMapsBucket).Delete(k); err != nil {
					return err
				}
			}
			return nil
		})
		if len(keys) < 1000 || err != nil {
			return
		}
	}
}

func (s3 *s3Storage) putSeq(name string) *atomic.Uint32 {
	return &s3.puts[maphash.String(missSeed, name)%uint64(len(s3.puts))]
}

// isMissing reports whether S3 is known to not have the key.
func (s3 *s3Storage) isMissing(name string) bool {
	if s3.misses == nil {
		return false
	}
	_, ok := s3.misses.Get(maphash.String(missSeed, name))
	return ok
}

// addMiss records a 404 of S3, seq is the put sequence of the key before the request.
func (s3 *s3Storage) addMiss(name string, seq uint32) {
	if s3.misses != nil {
		s3.misses.Add(maphash.String(missSeed, name), struct{}{})
		// [warning] check after the add, a put that ended in between has already removed its miss
		if s3.putSeq(name).Load() != seq {
			s3.misses.Remove(maphash.String(missSeed, name))
		}
	}
}

// markPut drops the miss of a key that has been written.
func (s3 *s3Storage) markPut(name string) {
	s3.putSeq(name).Add(1)
	if s3.misses != nil {
		s3.misses.Remove(maphash.String(missSeed, name))
	}
}

func (s3 *s3Storage) shouldUseMetaCache(name string) bool {
	return s3.metaCache != nil && isMetaKey(name)
}

func (s3 *s3Storage) getMetaCache(name string) (content []byte, stat Stat) {
	s3.metaCache.View(func(tx *bolt.Tx) error {
		if v := tx.Bucket(metaCacheBucket).Get([]byte(name)); len(v) >= 8 {
			content = bytes.Clone(v[8:])
			stat = &s3ObjectMeta{contentLength: int64(len(v) - 8), lastModified: time.UnixMilli(int64(binary.BigEndian.Uint64(v)))}
		}
		return nil
	})
	return
}

func (s3 *s3Storage) putMetaCache(name string, content []byte, modTime time.Time, epoch uint64) {
	value := binary.BigEndian.AppendUint64(make([]byte, 0, 8+len(content)), uint64(modTime.UnixMilli()))
	value = append(value, content...)
	s3.metaCache.Batch(func(tx *bolt.Tx) error {
		// [warning] check the epoch in the transaction, a delete commits its cache delete after the bump
		if s3.cacheEpoch.Load() != epoch {
			return nil
		}
		return tx.Bucket(metaCacheBucket).Put([]byte(name), value)
	})
}

func isMetaKey(name string) bool {
	return strings.HasPrefix(name, "meta/") || strings.HasPrefix(name, "meta-purged/")
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
