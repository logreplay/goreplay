/*
Package proto provides byte-level interaction with HTTP request payload.

Example of HTTP payload for future references, new line symbols escaped:

	POST /upload HTTP/1.1\r\n
	User-Agent: Gor\r\n
	Content-Length: 11\r\n
	\r\n
	Hello world

	GET /index.html HTTP/1.1\r\n
	User-Agent: Gor\r\n
	\r\n
	\r\n
*/
package proto

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/textproto"
	"strings"

	"goreplay/byteutils"
	"goreplay/logger"
)

const (
	// crlf In HTTP newline defined by 2 bytes (for both windows and *nix support)
	crlf = "\r\n"
	// emptyLine acts as separator: end of Headers or Body (in some cases)
	emptyLine = "\r\n\r\n"
	// headerDelim Separator for Header line. Header looks like: `HeaderName: value`
	headerDelim = ": "
)

// MIMEHeadersEndPos finds end of the Headers section, which should end with empty line.
func MIMEHeadersEndPos(payload []byte) int {
	pos := bytes.Index(payload, []byte(emptyLine))
	if pos < 0 {
		return -1
	}

	return pos + 4
}

// MIMEHeadersStartPos finds start of Headers section
// It just finds position of second line (first contains location and method).
func MIMEHeadersStartPos(payload []byte) int {
	pos := bytes.Index(payload, []byte(crlf))
	if pos < 0 {
		return -1
	}
	return pos + 2 // Find first line end
}

// header return value and positions of header/value start/end.
// If not found, value will be blank, and headerStart will be -1
// Do not support multi-line headers.
func header(payload []byte, name []byte) (value []byte, headerStart, headerEnd, valueStart, valueEnd int) {
	headerStart = MIMEHeadersStartPos(payload)
	if headerStart < 0 {
		return
	}

	var colonIndex int
	for headerStart < len(payload) {
		if headerEnd = bytes.IndexByte(payload[headerStart:], '\n'); headerEnd == -1 {
			break
		}

		headerEnd += headerStart
		if colonIndex = bytes.IndexByte(payload[headerStart:headerEnd], ':'); colonIndex == -1 {
			break
		}

		colonIndex += headerStart
		if bytes.EqualFold(payload[headerStart:colonIndex], name) {
			valueStart = colonIndex + 1
			valueEnd = headerEnd - 2
			break
		}

		headerStart = headerEnd + 1 // move to the next header
	}

	if valueStart == 0 {
		headerStart = -1
		headerEnd = -1
		valueEnd = -1
		valueStart = -1
		return
	}
	// ignore empty space after ':'
	for valueStart < valueEnd {
		if payload[valueStart] < 0x21 {
			valueStart++
		} else {
			break
		}
	}

	// ignore empty space at end of header value
	for valueEnd > valueStart {
		if payload[valueEnd] < 0x21 {
			valueEnd--
		} else {
			break
		}
	}

	value = payload[valueStart : valueEnd+1]

	return
}

// ParseHeaders Parsing headers from the payload
func ParseHeaders(p []byte) textproto.MIMEHeader {
	// trimming off the title of the request
	if HasTitle(p) {
		headerStart := MIMEHeadersStartPos(p)
		if headerStart > len(p)-1 {
			return nil
		}
		p = p[headerStart:]
	}
	headerEnd := MIMEHeadersEndPos(p)
	if headerEnd > 1 {
		p = p[:headerEnd]
	}
	return GetHeaders(p)
}

// GetHeaders returns mime headers from the payload
func GetHeaders(p []byte) textproto.MIMEHeader {
	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(p)))
	mime, err := reader.ReadMIMEHeader()
	if err != nil {
		return nil
	}
	return mime
}

// Header returns header value, if header not found, value will be blank
func Header(payload, name []byte) []byte {
	val, _, _, _, _ := header(payload, name)

	return val
}

// SetHeader sets header value. If header not found it creates new one.
// Returns modified request payload
func SetHeader(payload, name, value []byte) []byte {
	_, hs, _, vs, ve := header(payload, name)

	if hs != -1 {
		// If header found we just replace its value
		return byteutils.Replace(payload, vs, ve+1, value)
	}

	return AddHeader(payload, name, value)
}

// AddHeader takes http payload and appends new header to the start of headers section
// Returns modified request payload
func AddHeader(payload, name, value []byte) []byte {
	mimeStart := MIMEHeadersStartPos(payload)
	if mimeStart < 1 {
		return payload
	}

	header := make([]byte, len(name)+2+len(value)+2)
	copy(header[0:], name)
	copy(header[len(name):], headerDelim)
	copy(header[len(name)+2:], value)
	copy(header[len(header)-2:], crlf)

	return byteutils.Insert(payload, mimeStart, header)
}

// DeleteHeader takes http payload and removes header name from headers section
// Returns modified request payload
func DeleteHeader(payload, name []byte) []byte {
	_, hs, he, _, _ := header(payload, name)
	if hs != -1 {
		return byteutils.Cut(payload, hs, he+1)
	}

	return payload
}

// Body returns request/response body
func Body(payload []byte) []byte {
	pos := MIMEHeadersEndPos(payload)
	if pos == -1 || len(payload) <= pos {
		return nil
	}

	return payload[pos:]
}

// Path takes payload and retuns request path: Split(firstLine, ' ')[1]
func Path(payload []byte) []byte {
	if !HasTitle(payload) {
		return nil
	}

	start := bytes.IndexByte(payload, ' ') + 1
	end := bytes.IndexByte(payload[start:], ' ')

	return payload[start : start+end]
}

// SetPath takes payload, sets new path and returns modified payload
func SetPath(payload, path []byte) []byte {
	if !HasTitle(payload) {
		return nil
	}

	start := bytes.IndexByte(payload, ' ') + 1
	end := bytes.IndexByte(payload[start:], ' ')

	return byteutils.Replace(payload, start, start+end, path)
}

// PathParam returns URL query attribute by given name, if no found: valueStart will be -1
func PathParam(payload, name []byte) (value []byte, valueStart, valueEnd int) {
	path := Path(payload)

	var paramStart int
	if paramStart = bytes.Index(path, append([]byte{'&'}, append(name, '=')...)); paramStart == -1 {
		if paramStart = bytes.Index(path, append([]byte{'?'}, append(name, '=')...)); paramStart == -1 {
			return []byte(""), -1, -1
		}
	}

	valueStart = paramStart + len(name) + 2
	paramEnd := bytes.IndexByte(path[valueStart:], '&')

	// Param can end with '&' (another param), or end of line
	if paramEnd == -1 { // It is final param
		paramEnd = len(path)
	} else {
		paramEnd += valueStart
	}

	return path[valueStart:paramEnd], valueStart, paramEnd
}

// SetPathParam takes payload and updates path Query attribute
// If query param not found, it will append new
// Returns modified payload
func SetPathParam(payload, name, value []byte) []byte {
	path := Path(payload)
	_, vs, ve := PathParam(payload, name)

	if vs != -1 { // If param found, replace its value and set new Path
		newPath := make([]byte, len(path))
		copy(newPath, path)
		newPath = byteutils.Replace(newPath, vs, ve, value)

		return SetPath(payload, newPath)
	}

	// if param not found append to end of url
	// Adding 2 because of '?' or '&' at start, and '=' in middle
	newParam := make([]byte, len(name)+len(value)+2)

	if bytes.IndexByte(path, '?') == -1 {
		newParam[0] = '?'
	} else {
		newParam[0] = '&'
	}

	// Copy "param=value" into buffer, after it looks like "?param=value"
	copy(newParam[1:], name)
	newParam[1+len(name)] = '='
	copy(newParam[2+len(name):], value)

	// Append param to the end of path
	newPath := make([]byte, len(path)+len(newParam))
	copy(newPath, path)
	copy(newPath[len(path):], newParam)

	return SetPath(payload, newPath)
}

// SetHost updates Host header for HTTP/1.1 or updates host in path for HTTP/1.0 or Proxy requests
// Returns modified payload
func SetHost(payload, url, host []byte) []byte {
	// If this is HTTP 1.0 traffic or proxy traffic it may include host right into path variable,
	// so instead of setting Host header we rewrite Path
	// Fix for https://github.com/buger/gor/issues/156
	if path := Path(payload); bytes.HasPrefix(path, []byte("http")) {
		hostStart := bytes.IndexByte(path, ':') // : position "https?:"
		hostStart += 3                          // Skip 1 ':' and 2 '\'
		hostEnd := hostStart + bytes.IndexByte(path[hostStart:], '/')

		newPath := make([]byte, len(path))
		copy(newPath, path)
		newPath = byteutils.Replace(newPath, 0, hostEnd, url)

		return SetPath(payload, newPath)
	}

	return SetHeader(payload, []byte("Host"), host)
}

// Method returns HTTP method
func Method(payload []byte) []byte {
	end := bytes.IndexByte(payload, ' ')
	if end == -1 {
		return nil
	}

	return payload[:end]
}

// Status returns response status.
// It happens to be in same position as request payload path
func Status(payload []byte) []byte {
	return Path(payload)
}

// Methods holds the http methods ordered in ascending order
var Methods = [...]string{
	http.MethodConnect, http.MethodDelete, http.MethodGet,
	http.MethodHead, http.MethodOptions, http.MethodPatch,
	http.MethodPost, http.MethodPut, http.MethodTrace,
}

const (
	//MinRequestCount GET / HTTP/1.1\r\n
	MinRequestCount = 16
	// MinResponseCount HTTP/1.1 200 OK\r\n
	MinResponseCount = 17
	// VersionLen HTTP/1.1
	VersionLen = 8
)

// HasResponseTitle reports whether this payload has an HTTP/1 response title
func HasResponseTitle(payload []byte) bool {
	s := byteutils.SliceToString(payload)
	logger.Debug3("response:", s)
	if len(s) < MinResponseCount {
		return false
	}
	titleLen := bytes.Index(payload, []byte(crlf))
	if titleLen == -1 {
		logger.Debug3(fmt.Sprintf("paload titleLen count %d", titleLen))
		return false
	}
	major, minor, ok := http.ParseHTTPVersion(s[0:VersionLen])
	if !(checkHTTPVersion(major, minor, ok)) {
		logger.Debug3(fmt.Sprintf("http versin: %d.%d", major, minor))
		return false
	}
	if s[VersionLen] != ' ' {
		return false
	}
	status, ok := atoI(payload[VersionLen+1:VersionLen+4], 10)
	if !ok || s[VersionLen+4] != ' ' {
		logger.Debug3(fmt.Sprintf("parse status failed: %d", status))
		return false
	}
	statusText := http.StatusText(status)
	if statusText == "" {
		logger.Debug3(fmt.Sprintf("status text is empty: %d", status))
		return false
	}
	if titleLen+len(crlf) > len(s) {
		return false
	}
	logger.Debug3(fmt.Sprintf("'%s' == '%s'", string(payload[VersionLen+4]), string(payload[VersionLen+4])))
	// handle cases from #875
	return payload[VersionLen+4] == ' ' || payload[VersionLen+4] == '\r'
}

// HasRequestTitle reports whether this payload has an HTTP/1 request title
func HasRequestTitle(payload []byte) bool {
	s := byteutils.SliceToString(payload)
	if len(s) < MinRequestCount {
		return false
	}
	titleLen := bytes.Index(payload, []byte(crlf))
	if titleLen == -1 {
		return false
	}
	if strings.Count(s[:titleLen], " ") != 2 {
		return false
	}
	method := string(Method(payload))
	var methodFound bool
	for _, m := range Methods {
		if methodFound = method == m; methodFound {
			break
		}
	}
	if !methodFound {
		return false
	}
	path := strings.Index(s[len(method)+1:], " ")
	if path == -1 {
		return false
	}
	major, minor, ok := http.ParseHTTPVersion(s[path+len(method)+2 : titleLen])
	return checkHTTPVersion(major, minor, ok)
}

// HasTitle reports if this payload has an http/1 title
func HasTitle(payload []byte) bool {
	return HasRequestTitle(payload) || HasResponseTitle(payload)
}

// checkHTTPVersion
func checkHTTPVersion(major int, minor int, ok bool) bool {
	if ok && major == 1 && (minor == 0 || minor == 1) {
		return true
	}

	return false
}

// CheckChunked checks HTTP/1 chunked data integrity and return the final index
// of chunks(index after '0\r\n\r\n') or -1 if there is missing data
// or there is bad format
func CheckChunked(buf []byte) (chunkEnd int) {
	var (
		ok     bool
		chkLen int
		sz     int
		ext    int
	)
	logger.Debug3("chunk buf:", string(buf))
	for {
		sz = bytes.IndexByte(buf[chunkEnd:], '\r')
		if sz < 1 {
			logger.Debug3("chunk is not end of \\r")
			return -1
		}
		// ignoring chunks extensions https://github.com/golang/go/issues/13135
		// but chunks extensions are no longer a thing
		ext = bytes.IndexByte(buf[chunkEnd:chunkEnd+sz], ';')
		if ext < 0 {
			ext = sz
		}

		chkLen, ok = atoI(buf[chunkEnd:chunkEnd+ext], 16)
		if !ok {
			return -1
		}

		chunkEnd += sz + 2
		if chkLen == 0 {
			if !bytes.Equal(buf[chunkEnd:chunkEnd+2], []byte(crlf)) {
				return -1
			}
			return chunkEnd + 2
		}
		// ideally chunck length and at least len("\r\n0\r\n\r\n")
		if len(buf[chunkEnd:]) < chkLen+7 {
			return -1
		}

		chunkEnd += chkLen
		// chunks must end with crlf
		if !bytes.Equal(buf[chunkEnd:chunkEnd+2], []byte(crlf)) {
			return -1
		}

		chunkEnd += 2
	}
}

// Feedback is an interface used to provide feedback or store dummy data for future use
type Feedback interface {
	SetFeedback(interface{})
	Feedback() interface{}
}

type feedback struct {
	body     int // body index
	hdrStart int
	headers  textproto.MIMEHeader
}

// HasFullPayload reports if this http has full payloads
func HasFullPayload(data []byte, f Feedback) bool {
	var feed *feedback
	var ok bool
	var body []byte
	feed, ok = generateFeedback(data, f)
	if !ok {
		return false
	}

	if len(data) > feed.body {
		body = data[feed.body:]
	}

	if feed.headers.Get("Transfer-Encoding") == "chunked" {
		return checkChunks(body, feed.headers)
	}

	// check for content-length header
	if header := feed.headers.Get("Content-Length"); header != "" {
		num, ok := atoI([]byte(header), 10)
		// trailers are generally not allowed in non-chunks body
		return ok && num == len(body)
	}

	return true
}

// generateFeedback generate *feedback struct
func generateFeedback(data []byte, f Feedback) (*feedback, bool) {
	var feed *feedback
	var ok bool
	if f != nil {
		feed, ok = f.Feedback().(*feedback)
	}

	if !ok || feed == nil {
		feed = new(feedback)
	}

	if f != nil {
		f.SetFeedback(feed)
	}

	if feed.hdrStart < 1 {
		if feed.hdrStart = MIMEHeadersStartPos(data); feed.hdrStart < 0 {
			return nil, false
		}
	}

	if feed.body < 1 {
		if feed.body = MIMEHeadersEndPos(data); feed.body < 0 {
			return nil, false
		}
	}

	if feed.headers == nil {
		if feed.headers = GetHeaders(data[feed.hdrStart:feed.body]); feed.headers == nil {
			return nil, false
		}
	}

	return feed, true
}

// check chunks
func checkChunks(body []byte, headers textproto.MIMEHeader) bool {
	if len(body) < 1 {
		return false
	}

	var chunkEnd int
	if chunkEnd = CheckChunked(body); chunkEnd < 1 {
		return false
	}
	// check trailer headers
	if headers.Get("Trailer") == "" {
		return true
	}
	// trailer headers(whether chunked or plain) should end with empty line
	return len(body) > chunkEnd && MIMEHeadersEndPos(body[chunkEnd:]) != -1
}

// this works with positive integers
func atoI(s []byte, base int) (num int, ok bool) {
	var v int
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return 0, false
		}

		v = int(hexTable[s[i]])
		if v >= base || (v == 0 && s[i] != '0') {
			return 0, false
		}

		num = (num * base) + v
	}

	return num, true
}

var hexTable = [128]byte{
	'0': 0,
	'1': 1,
	'2': 2,
	'3': 3,
	'4': 4,
	'5': 5,
	'6': 6,
	'7': 7,
	'8': 8,
	'9': 9,
	'A': 10,
	'a': 10,
	'B': 11,
	'b': 11,
	'C': 12,
	'c': 12,
	'D': 13,
	'd': 13,
	'E': 14,
	'e': 14,
	'F': 15,
	'f': 15,
}
