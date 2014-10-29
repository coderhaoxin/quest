package quest

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	. "github.com/go-libs/methods"
)

type JSONMaps map[string]interface{}

type HandlerFunc func(*http.Request, *http.Response, interface{}, error)
type BytesHandlerFunc func(*http.Request, *http.Response, []byte, error)
type StringHandlerFunc func(*http.Request, *http.Response, string, error)
type JSONHandlerFunc func(*http.Request, *http.Response, JSONMaps, error)

type Qrequest struct {
	Method Method
	Url    string
	Uri    *url.URL
	req    *http.Request
	res    *http.Response
	client *http.Client

	// request header & body
	Header http.Header
	Body   io.ReadCloser
	Length int64

	isBodyClosed bool
	Buffer       *bytes.Buffer

	err error
}

func (r *Qrequest) Query(data interface{}) *Qrequest {
	var queryString string
	switch t := data.(type) {
	case url.Values:
		queryString = t.Encode()
		break
	case string:
		queryString = t
		break
	case []byte:
		queryString = string(t)
		break
	}
	r.Uri.RawQuery = queryString
	return r
}

func (r *Qrequest) Parameters(data interface{}) *Qrequest {
	if encodesParametersInURL(r.Method) {
		return r
	}
	var (
		body   io.ReadCloser
		length int64
	)
	switch t := data.(type) {
	case url.Values:
		body, length = packBodyByString(t.Encode())
		break
	case string:
		body, length = packBodyByString(t)
		break
	case []byte:
		body, length = packBodyByBytes(t)
		break
	case *bytes.Buffer:
		body, length = packBodyByBytesBuffer(t)
		break
	case *bytes.Reader:
		body, length = packBodyByBytesReader(t)
		break
	case *strings.Reader:
		body, length = packBodyByStringsReader(t)
		break
		// JSON Object
	default:
		b, err := json.Marshal(data)
		if err != nil {
			r.err = err
			return r
		}
		body, length = packBodyByBytes(b)
	}
	if body != nil {
		r.Body = body
		r.Length = length
	}
	return r
}

func (r *Qrequest) Encoding(t string) *Qrequest {
	t = strings.ToUpper(t)
	if t == "JSON" {
		t = "application/json"
	}
	if t != "" {
		r.Header.Set("Content-Type", t)
	}
	return r
}

func (r *Qrequest) Authenticate(username, password string) *Qrequest {
	return r
}

func (r *Qrequest) Progress() *Qrequest {
	return r
}

func (r *Qrequest) response() (*bytes.Buffer, error) {
	if r.err != nil {
		return r.Buffer, r.err
	}
	if r.isBodyClosed {
		return r.Buffer, nil
	}
	r.isBodyClosed = true
	return r.Do()
}

func (r *Qrequest) Response(handler HandlerFunc) *Qrequest {
	body, err := r.response()
	handler(r.req, r.res, body, err)
	return r
}

func (r *Qrequest) ResponseBytes(handler BytesHandlerFunc) *Qrequest {
	body, err := r.response()
	handler(r.req, r.res, body.Bytes(), err)
	return r
}

func (r *Qrequest) ResponseString(handler StringHandlerFunc) *Qrequest {
	body, err := r.response()
	handler(r.req, r.res, body.String(), err)
	return r
}

func (r *Qrequest) ResponseJSON(handler JSONHandlerFunc) *Qrequest {
	body, err := r.response()
	if err != nil {
		handler(r.req, r.res, nil, err)
	} else {
		data := JSONMaps{}
		err = json.Unmarshal(body.Bytes(), &data)
		handler(r.req, r.res, data, err)
	}
	return r
}

func (r *Qrequest) Validate() *Qrequest {
	return r
}

func (r *Qrequest) validateAcceptContentType(map[string]string) bool {
	return true
}

// Acceptable Content Type
func (r *Qrequest) ValidateAcceptContentType(map[string]string) bool {
	return true
}

func (r *Qrequest) validateStatusCode(statusCodes ...int) bool {
	statusCode := r.res.StatusCode
	if len(statusCodes) > 0 {
		for _, c := range statusCodes {
			if statusCode == c {
				return true
			}
		}
		// 200 <= x < 300
	} else if statusCode >= 200 && statusCode < 300 {
		return true
	}
	return false
}

// Status Code
func (r *Qrequest) ValidateStatusCode(statusCodes ...int) *Qrequest {
	r.response()
	if !r.validateStatusCode(statusCodes...) {
		r.err = errors.New("http: invalid status code " + strconv.Itoa(r.res.StatusCode))
	}
	return r
}

func (r *Qrequest) Cancel() {}

func (r *Qrequest) Do() (*bytes.Buffer, error) {
	r.req = &http.Request{
		Method: r.Method.String(),
		URL:    r.Uri,
		Header: r.Header,
	}
	if r.req.Header.Get("Content-Type") == "" {
		r.req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if r.Body != nil {
		r.req.Body = r.Body
		r.req.ContentLength = r.Length
	}
	r.client = &http.Client{}
	res, err := r.client.Do(r.req)
	if err != nil {
		return nil, err
	}
	r.res = res
	defer res.Body.Close()
	r.Buffer = new(bytes.Buffer)
	r.Buffer.ReadFrom(res.Body)
	return r.Buffer, nil
}

// Helpers:

func packBodyByString(s string) (io.ReadCloser, int64) {
	return ioutil.NopCloser(bytes.NewBufferString(s)), int64(len(s))
}

func packBodyByBytes(b []byte) (io.ReadCloser, int64) {
	return ioutil.NopCloser(bytes.NewBuffer(b)), int64(len(b))
}

func packBodyByBytesBuffer(b *bytes.Buffer) (io.ReadCloser, int64) {
	return ioutil.NopCloser(b), int64(b.Len())
}

func packBodyByBytesReader(b *bytes.Reader) (io.ReadCloser, int64) {
	return ioutil.NopCloser(b), int64(b.Len())
}

func packBodyByStringsReader(b *strings.Reader) (io.ReadCloser, int64) {
	return ioutil.NopCloser(b), int64(b.Len())
}

func encodesParametersInURL(method Method) bool {
	switch method {
	case GET, HEAD, DELETE:
		return true
	default:
		return false
	}
}

func escape(s string) string {
	return url.QueryEscape(s)
}
