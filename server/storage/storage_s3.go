package storage

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
	return &s3Storage{
		apiEndpoint:     strings.TrimSuffix(u.String(), "/"),
		region:          options.Region,
		accessKeyID:     options.AccessKeyID,
		secretAccessKey: options.SecretAccessKey,
	}, nil
}

// A S3-compatible storage.
type s3Storage struct {
	apiEndpoint     string
	region          string
	accessKeyID     string
	secretAccessKey string
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

func parseS3Error(resp *http.Response) s3Error {
	var s3Error s3Error
	if xml.NewDecoder(resp.Body).Decode(&s3Error) != nil {
		s3Error.Code = "error"
		s3Error.Message = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
	}
	return s3Error
}

func (e s3Error) Error() string {
	return e.Code + ": " + e.Message
}

func (s3 *s3Storage) Stat(name string) (stat Stat, err error) {
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
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return
	}
	contentLengthHeader := resp.Header.Get("Content-Length")
	lastModifiedHeader := resp.Header.Get("Last-Modified")
	if contentLengthHeader == "" {
		err = errors.New("missing content size header")
		return
	}
	if lastModifiedHeader == "" {
		err = errors.New("missing last modified header")
		return
	}
	size, _ := strconv.ParseInt(contentLengthHeader, 10, 64)
	lastModified, _ := time.Parse(time.RFC1123, lastModifiedHeader)
	return &s3ObjectMeta{
		contentLength: size,
		lastModified:  lastModified,
	}, nil
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

func (s3 *s3Storage) Get(name string) (content io.ReadCloser, stat Stat, err error) {
	if name == "" {
		return nil, nil, errors.New("name is required")
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
	lastModifiedHeader := resp.Header.Get("Last-Modified")
	if contentLengthHeader == "" {
		err = errors.New("missing content size header")
		return
	}
	if lastModifiedHeader == "" {
		err = errors.New("missing last modified header")
		return
	}
	size, _ := strconv.ParseInt(contentLengthHeader, 10, 64)
	lastModified, _ := time.Parse(time.RFC1123, lastModifiedHeader)
	return resp.Body, &s3ObjectMeta{
		contentLength: size,
		lastModified:  lastModified,
	}, nil
}

func (s3 *s3Storage) Put(name string, content io.Reader) (err error) {
	if name == "" {
		return errors.New("name is required")
	}
	req, _ := http.NewRequest("PUT", s3.apiEndpoint+"/"+name, content)
	s3.sign(req)
	if buf, ok := content.(*bytes.Buffer); ok {
		req.ContentLength = int64(buf.Len())
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
		req.ContentLength = size
	}
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

func (s3 *s3Storage) Delete(keys ...string) (err error) {
	if len(keys) == 0 {
		return nil
	} else if len(keys) == 1 {
		req, _ := http.NewRequest("DELETE", s3.apiEndpoint+"/"+keys[0], nil)
		s3.sign(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return errors.New("unexpected status code: " + resp.Status)
		}
	} else {
		buf := new(bytes.Buffer)
		buf.WriteString("<Delete>")
		for _, key := range keys {
			buf.WriteString("<Object><Key>")
			buf.WriteString(html.EscapeString(key))
			buf.WriteString("</Key></Object>")
		}
		buf.WriteString("<Quiet>true</Quiet>")
		buf.WriteString("</Delete>")
		req, _ := http.NewRequest("POST", s3.apiEndpoint+"?delete", buf)
		s3.sign(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return parseS3Error(resp)
		}
	}
	return nil
}

func (s3 *s3Storage) DeleteAll(prefix string) (deletedKeys []string, err error) {
	if prefix == "" {
		return nil, errors.New("prefix is required")
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
	canonicalRequest := strings.Join([]string{req.Method, escapePath(req.URL.EscapedPath()), req.URL.Query().Encode(), strings.Join(canonicalHeaders, "\n") + "\n", strings.Join(signedHeaders, ";"), req.Header.Get("X-Amz-Content-Sha256")}, "\n")
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", datetime, scope, toHex(sha256Sum(canonicalRequest))}, "\n")
	signingKey := hmacSum(hmacSum(hmacSum(hmacSum([]byte("AWS4"+s3.secretAccessKey), date), s3.region), "s3"), "aws4_request")
	signature := hmacSum(signingKey, stringToSign)
	req.Header.Set("Authorization", strings.Join([]string{"AWS4-HMAC-SHA256 Credential=" + s3.accessKeyID + "/" + scope, "SignedHeaders=" + strings.Join(signedHeaders, ";"), "Signature=" + toHex(signature)}, ", "))
}
