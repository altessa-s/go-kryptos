// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"path"

	kryptos "github.com/altessa-s/go-kryptos"
)

// write observes the HTTP outcome before decoding metadata. Mutations must not
// inherit the SDK's automatic retries after an ambiguous transport failure.
func (s *Storage[T]) write(ctx context.Context, key string, data map[string]any) error {
	client, err := s.client.CloneWithHeaders()
	if err != nil {
		return kryptos.ErrInvalidOptions
	}
	client.SetMaxRetries(0)
	request := s.client.NewRequest(http.MethodPut, "/v1/"+path.Join(s.o.mountPath, "data", key))
	if err = request.SetJSONBody(data); err != nil {
		return kryptos.ErrValueEncoding
	}
	defer clear(request.BodyBytes)
	//nolint:staticcheck // Raw status is required to distinguish commitment from decoding failure.
	response, err := client.RawRequestWithContext(ctx, request)
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	if response == nil || response.Response == nil {
		return kryptos.ErrIndeterminate
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if response.StatusCode >= http.StatusInternalServerError || err == nil {
			return kryptos.ErrIndeterminate
		}
		return classify(err)
	}
	var body struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if response.Body == nil || json.NewDecoder(response.Body).Decode(&body) != nil || body.Data.Version <= 0 {
		return &kryptos.CommittedError{Err: kryptos.ErrMalformed}
	}
	return nil
}

// deleteMetadata never replays an ambiguous deletion: a new secret may already
// occupy the same path when a retry reaches the backend.
func (s *Storage[T]) deleteMetadata(ctx context.Context, key string) error {
	client, err := s.client.CloneWithHeaders()
	if err != nil {
		return kryptos.ErrInvalidOptions
	}
	client.SetMaxRetries(0)
	// Build from the original client to retain authentication and request settings.
	request := s.client.NewRequest(http.MethodDelete, "/v1/"+path.Join(s.o.mountPath, "metadata", key))
	//nolint:staticcheck // Raw status distinguishes definitive rejection from an unknown mutation outcome.
	response, err := client.RawRequestWithContext(ctx, request)
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	if response == nil || response.Response == nil {
		return kryptos.ErrIndeterminate
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	if response.StatusCode >= http.StatusInternalServerError || err == nil {
		return kryptos.ErrIndeterminate
	}
	return classify(err)
}
