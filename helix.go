package helix

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

const (
	basePath  = "https://api.twitch.tv/helix"
	methodGet = "GET"
	queryTag  = "query"
)

// HTTPClient ...
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client ...
type Client struct {
	clientID    string
	accessToken string
	userAgent   string
	httpClient  HTTPClient
}

// ResponseCommon ...
type ResponseCommon struct {
	StatusCode   int
	Error        string `json:"error"`
	ErrorStatus  int    `json:"status"`
	ErrorMessage string `json:"message"`
	Ratelimit
}

// Response ...
type Response struct {
	ResponseCommon
	Data interface{}
}

// Ratelimit ...
type Ratelimit struct {
	RatelimitLimit     int
	RatelimitRemaining int
	RatelimitReset     int64
}

// Pagination ...
type Pagination struct {
	Cursor string `json:"cursor"`
}

// NewClient ... It is concurrecy safe.
func NewClient(clientID string, httpClient HTTPClient) (*Client, error) {
	c := &Client{}
	c.clientID = clientID

	if c.clientID == "" {
		return nil, errors.New("clientID cannot be an empty string")
	}

	if httpClient != nil {
		c.httpClient = httpClient
	} else {
		c.httpClient = http.DefaultClient
	}

	return c, nil
}

func (c *Client) get(path string, data, params interface{}) (*Response, error) {
	resp := &Response{
		Data: data,
	}

	req, err := c.newRequest(methodGet, path, params)
	if err != nil {
		return nil, err
	}

	err = c.doRequest(req, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func buildQueryString(req *http.Request, v interface{}) (string, error) {
	isNil, err := isZero(v)
	if err != nil {
		return "", err
	}

	if isNil {
		return "", nil
	}

	query := req.URL.Query()
	t := reflect.TypeOf(v).Elem()
	val := reflect.ValueOf(v).Elem()

	for i := 0; i < t.NumField(); i++ {
		var defaultValue string

		field := t.Field(i)
		tag := field.Tag.Get(queryTag)

		// Get the default value from the struct tag
		if strings.Contains(tag, ",") {
			tagSlice := strings.Split(tag, ",")

			tag = tagSlice[0]
			defaultValue = tagSlice[1]
		}

		// Get the value assigned to the query param
		if field.Type.Kind() == reflect.Slice {
			fieldVal := val.Field(i)
			for j := 0; j < fieldVal.Len(); j++ {
				query.Add(tag, fmt.Sprintf("%v", fieldVal.Index(j)))
			}
		} else {
			value := fmt.Sprintf("%v", val.Field(i))

			// If no value was set by the user, use the default
			// value specified in the struct tag.
			if value == "" || value == "0" {
				if defaultValue == "" {
					continue
				}

				value = defaultValue
			}

			query.Add(tag, value)
		}
	}

	return query.Encode(), nil
}

func isZero(v interface{}) (bool, error) {
	t := reflect.TypeOf(v)
	if !t.Comparable() {
		return false, fmt.Errorf("type is not comparable: %v", t)
	}
	return v == reflect.Zero(t).Interface(), nil
}

func (c *Client) newRequest(method, path string, params interface{}) (*http.Request, error) {
	url := concatString([]string{basePath, path})

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	if method == methodGet && params != nil {
		query, err := buildQueryString(req, params)
		if err != nil {
			return nil, err
		}

		req.URL.RawQuery = query
	}

	return req, nil
}

func (c *Client) doRequest(req *http.Request, resp *Response) error {
	c.setRequestHeaders(req)

	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to execute API request: %s", err.Error())
	}
	defer response.Body.Close()

	setResponseStatusCode(resp, "StatusCode", response.StatusCode)
	setRatelimitValue(resp, "RatelimitLimit", response.Header.Get("Ratelimit-Limit"))
	setRatelimitValue(resp, "RatelimitRemaining", response.Header.Get("Ratelimit-Remaining"))
	setRatelimitValue(resp, "RatelimitReset", response.Header.Get("Ratelimit-Reset"))

	// Only attempt to decode the response if JSON was returned
	if resp.StatusCode < 500 {
		decoder := json.NewDecoder(response.Body)
		if resp.StatusCode < 400 {
			// Successful request
			err = decoder.Decode(&resp.Data)

		} else {
			// Failed request
			err = decoder.Decode(resp)
		}

		if err != nil {
			return fmt.Errorf("Failed to decode API response: %s", err.Error())
		}
	}

	return nil
}

func (c *Client) setRequestHeaders(req *http.Request) {
	req.Header.Set("Client-ID", c.clientID)
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
}

func setResponseStatusCode(v interface{}, fieldName string, code int) {
	s := reflect.ValueOf(v).Elem()
	field := s.FieldByName(fieldName)
	field.SetInt(int64(code))
}

func setRatelimitValue(v interface{}, fieldName, value string) {
	s := reflect.ValueOf(v).Elem()
	field := s.FieldByName(fieldName)
	intVal, _ := strconv.Atoi(value)
	field.SetInt(int64(intVal))
}

// SetAccessToken ...
func (c *Client) SetAccessToken(AccessToken string) {
	c.accessToken = AccessToken
}

// SetUserAgent ...
func (c *Client) SetUserAgent(userAgent string) {
	c.userAgent = userAgent
}

// concatString concatenates each of the strings provided by
// strs in the order they are presented. You may also pass in
// an optional delimiter to be appended along with the strings.
func concatString(strs []string, delimiter ...string) string {
	var buffer bytes.Buffer
	appendDelimiter := len(delimiter) > 0

	for _, str := range strs {
		s := str
		if appendDelimiter {
			s = concatString([]string{s, delimiter[0]})
		}
		buffer.Write([]byte(s))
	}

	if appendDelimiter {
		return strings.TrimSuffix(buffer.String(), delimiter[0])
	}

	return buffer.String()
}