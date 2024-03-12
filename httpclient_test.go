package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testPayload struct {
	Status string `json:"status"`
}

//nolint:funlen
func TestClient(t *testing.T) {
	ctx := context.Background()

	var p struct {
		Target struct {
			Name string `json:"name"`
		} `json:"target"`
	}
	p.Target.Name = "test"

	payload, _ := json.Marshal(p)

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	mock := httpmock.NewMockTransport()

	headers := http.Header{}
	headers.Add("location", "https://this.location/")

	r := httpmock.NewStringResponder(http.StatusOK, `{"status":"OK"}`)
	r = r.HeaderAdd(headers)

	mock.RegisterResponder(http.MethodPost, "https://my.path/test", r)
	mock.RegisterResponder(http.MethodGet, "https://my.path/test", r)
	mock.RegisterResponder(http.MethodPut, "https://my.path/test", r)
	mock.RegisterResponder(http.MethodDelete, "https://my.path/test", r)
	mock.RegisterResponder(http.MethodHead, "https://my.path/test", r)
	mock.RegisterResponder(http.MethodPatch, "https://my.path/test", r)

	client := New(&http.Client{Transport: mock}, &Options{})

	var err error

	expected := testPayload{Status: "OK"}

	result := testPayload{}
	h := http.Header{}
	// Post
	err = client.Post(ctx, "https://my.path/test", payload, &result, &h)
	assert.NoError(t, err)
	assert.Equal(t, "https://this.location/", h.Get("location"))
	assert.EqualValues(t, expected, result)

	err = client.Post(ctx, "https://my.path/test", payload, nil, &h)
	assert.NoError(t, err)
	assert.Equal(t, "https://this.location/", h.Get("location"), "should still retrieve headers with nil payload")

	err = client.Post(ctx, "https://my.path/test", payload, &result, nil)
	assert.NoError(t, err)
	assert.EqualValues(t, expected, result)

	// Get
	result = testPayload{}
	h = http.Header{}
	h.Add("content-type", "application/json")
	err = client.Get(ctx, "https://my.path/test", &result, &h)
	assert.NoError(t, err)
	assert.Equal(t, "https://this.location/", h.Get("location"))
	assert.EqualValues(t, expected, result)

	err = client.Get(ctx, "https://my.path/test", &result, nil)
	assert.NoError(t, err)
	assert.EqualValues(t, expected, result)

	// Head
	h = http.Header{}
	err = client.Head(ctx, "https://my.path/test", payload, &h)
	assert.NoError(t, err)
	assert.Equal(t, "https://this.location/", h.Get("location"))

	// Put
	result = testPayload{}
	h = http.Header{}
	err = client.Put(ctx, "https://my.path/test", payload, &result, &h)
	assert.NoError(t, err)

	// Patch
	result = testPayload{}
	h = http.Header{}
	err = client.Patch(ctx, "https://my.path/test", payload, &result, &h)
	assert.NoError(t, err)

	// Delete
	result = testPayload{}
	h = http.Header{}
	err = client.Delete(ctx, "https://my.path/test", payload, &result, &h)
	assert.NoError(t, err)
}

func TestSendHeaders(t *testing.T) {
	ctx := context.Background()

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	mock := httpmock.NewMockTransport()

	mock.RegisterResponder(http.MethodGet, "https://my.path/test",
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, req.Header.Get("test"), "testvalue")
			resp, _ := httpmock.NewStringResponder(http.StatusOK, `{ "data":{"Random": "Stuff"}}`)(req)
			return resp, nil
		})

	client := New(&http.Client{Transport: mock}, &Options{})

	headers := http.Header{}
	headers.Add("test", "testvalue")

	err := client.Get(ctx, "https://my.path/test", nil, &headers)
	assert.NoError(t, err)

	mock.RegisterResponder(http.MethodPost, "https://my.path/test",
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, req.Header.Get("test"), "testvalue")
			resp, _ := httpmock.NewStringResponder(http.StatusOK, `{ "data":{"Random": "Stuff"}}`)(req)
			return resp, nil
		})

	headers = http.Header{}
	headers.Add("test", "testvalue")

	err = client.Post(ctx, "https://my.path/test", nil, nil, &headers)
	assert.NoError(t, err)
}

func TestDo(t *testing.T) {
	tests := []struct {
		u string
		r int
		e error
	}{
		{
			u: "/notfound",
			r: http.StatusNotFound,
			e: ErrNotFound,
		},
		{
			u: "/unauthorized",
			r: http.StatusUnauthorized,
			e: ErrUserAccessDenied,
		},
		{
			u: "/forbidden",
			r: http.StatusForbidden,
			e: ErrUserAccessDenied,
		},
		{
			u: "/toomanyrequests",
			r: http.StatusTooManyRequests,
			e: ErrTooManyRequests,
		},
		{
			u: "/unprocessableentity",
			r: http.StatusUnprocessableEntity,
			e: ErrUnprocessableEntity,
		},
		{
			u: "/internalservererror",
			r: http.StatusInternalServerError,
			e: ErrInternalServerError,
		},
		{
			u: "/badgateway",
			r: http.StatusBadGateway,
			e: ErrBadGateway,
		},
		{
			u: "/serviceunavailable",
			r: http.StatusServiceUnavailable,
			e: ErrServiceUnavailable,
		},
		{
			u: "/gatewaytimeout",
			r: http.StatusGatewayTimeout,
			e: ErrGatewayTimeout,
		},
		{
			u: "/badrequest",
			r: http.StatusBadRequest,
			e: ErrBadRequest,
		},
		{
			u: "/gone",
			r: http.StatusGone,
			e: ErrUnhandled,
		},
	}

	require.NotPanics(t, func() { New(nil, nil) })

	c := New(http.DefaultClient, &Options{})

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, test := range tests {
			if r.URL.String() == test.u {
				w.WriteHeader(test.r)
				break
			}
		}
	}))

	for _, test := range tests {
		r, err := http.NewRequest(http.MethodGet, s.URL+test.u, http.NoBody)
		require.NoError(t, err)
		_, err = c.do(r) //nolint

		require.ErrorIs(t, err, test.e)
	}
}
