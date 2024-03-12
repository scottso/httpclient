package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
)

const failMsg = "failed to create %s request: %w"

var (
	ErrUserAccessDenied    = errors.New("requested resource unauthorized")
	ErrNotFound            = errors.New("requested resource not found")
	ErrTooManyRequests     = errors.New("requested resource rate limited")
	ErrNilResponse         = errors.New("response is nil")
	ErrUnprocessableEntity = errors.New("unprocessable entity")
	ErrInternalServerError = errors.New("internal server error")
	ErrBadRequest          = errors.New("bad request")
	ErrBadGateway          = errors.New("bad gateway")
	ErrServiceUnavailable  = errors.New("service unavailable")
	ErrGatewayTimeout      = errors.New("gateway timeout")
	ErrUnhandled           = errors.New("unknown error")

	// Be opinionated about which errors are retriable.  Used to wrap errors we
	// consider transient.
	ErrRetriable = errors.New("retriable error")
)

type Options struct {
	Debug bool
}

type Client struct {
	httpClient *http.Client
	options    *Options
}

func (c *Client) Head(ctx context.Context, apiURL string, payload []byte, headers *http.Header) error {
	req, err := c.newRequest(ctx, http.MethodHead, apiURL, payload)
	if err != nil {
		return fmt.Errorf(failMsg, http.MethodHead, err)
	}

	if err := c.doRequest(req, nil, headers); err != nil {
		return err
	}

	return nil
}

func (c *Client) Get(ctx context.Context, apiURL string, v any, headers *http.Header) error {
	req, err := c.newRequest(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf(failMsg, http.MethodGet, err)
	}

	if err := c.doRequest(req, v, headers); err != nil {
		return err
	}

	return nil
}

func (c *Client) Post(ctx context.Context, apiURL string, payload []byte, v any, headers *http.Header) error {
	req, err := c.newRequest(ctx, http.MethodPost, apiURL, payload)
	if err != nil {
		return fmt.Errorf(failMsg, http.MethodPost, err)
	}

	if err := c.doRequest(req, v, headers); err != nil {
		return err
	}

	return nil
}

func (c *Client) Put(ctx context.Context, apiURL string, payload []byte, v any, headers *http.Header) error {
	req, err := c.newRequest(ctx, http.MethodPut, apiURL, payload)
	if err != nil {
		return fmt.Errorf(failMsg, http.MethodPut, err)
	}

	if err := c.doRequest(req, v, headers); err != nil {
		return err
	}

	return nil
}

func (c *Client) Patch(ctx context.Context, apiURL string, payload []byte, v any, headers *http.Header) error {
	req, err := c.newRequest(ctx, http.MethodPatch, apiURL, payload)
	if err != nil {
		return fmt.Errorf(failMsg, http.MethodPatch, err)
	}

	if err := c.doRequest(req, v, headers); err != nil {
		return err
	}

	return nil
}

func (c *Client) Delete(ctx context.Context, apiURL string, payload []byte, v any, headers *http.Header) error {
	req, err := c.newRequest(ctx, http.MethodDelete, apiURL, payload)
	if err != nil {
		return fmt.Errorf(failMsg, http.MethodDelete, err)
	}

	if err := c.doRequest(req, v, headers); err != nil {
		return err
	}

	return nil
}

func (c *Client) newRequest(ctx context.Context, method, apiURL string, payload []byte) (*http.Request, error) {
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, apiURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	if c.options.Debug {
		body, _ := httputil.DumpRequest(req, true)
		log.Printf("%s", body)
	}

	req = req.WithContext(ctx)
	return req, nil
}

func (c *Client) doRequest(r *http.Request, v any, headers *http.Header) error {
	if headers != nil {
		r.Header = *headers
	}

	resp, err := c.do(r)
	if err != nil {
		return err
	}

	if resp == nil {
		return ErrNilResponse
	}
	defer resp.Body.Close()

	if headers != nil {
		*headers = resp.Header.Clone()
	}

	if v == nil {
		// Drain the body so the connection can be reused
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	var buf bytes.Buffer
	dec := json.NewDecoder(io.TeeReader(resp.Body, &buf))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf(
			"could not parse response body: %w [%s:%s] %s",
			err,
			r.Method,
			r.URL.String(),
			buf.String(),
		)
	}

	return nil
}

func (c *Client) do(r *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to make request [%s:%s]: %w", r.Method, r.URL.String(), err)
	}

	if c.options.Debug {
		body, _ := httputil.DumpResponse(resp, true)
		log.Printf("%s", body)
	}

	switch resp.StatusCode {
	case http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNoContent:
		return resp, nil
	}

	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	// if we get here there was an error
	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUserAccessDenied
	case http.StatusTooManyRequests:
		return nil, errors.Join(ErrTooManyRequests, ErrRetriable)
	case http.StatusUnprocessableEntity:
		return nil, errors.Join(ErrUnprocessableEntity, ErrRetriable)
	case http.StatusInternalServerError:
		return nil, errors.Join(ErrInternalServerError, ErrRetriable)
	case http.StatusBadGateway:
		return nil, errors.Join(ErrBadGateway, ErrRetriable)
	case http.StatusServiceUnavailable:
		return nil, errors.Join(ErrServiceUnavailable, ErrRetriable)
	case http.StatusGatewayTimeout:
		return nil, errors.Join(ErrGatewayTimeout, ErrRetriable)
	case http.StatusBadRequest:
		return nil, ErrBadRequest
	}

	return nil, errors.Join(fmt.Errorf("request failed, %d status code received: %s", resp.StatusCode, b), ErrUnhandled)
}

func New(httpClient *http.Client, options *Options) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	if options == nil {
		options = &Options{}
	}

	return &Client{
		httpClient: httpClient,
		options:    options,
	}
}
