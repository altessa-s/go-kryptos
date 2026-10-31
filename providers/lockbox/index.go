// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kryptos "github.com/altessa-s/go-kryptos"
	pb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
)

func (s *Storage[T]) secrets(ctx context.Context) ([]*pb.Secret, error) {
	var all []*pb.Secret
	page := ""
	seen := map[string]bool{}
	for {
		r, e := s.ss.List(ctx, &pb.ListSecretsRequest{FolderId: s.folder, PageToken: page, PageSize: listPageSize})
		if e != nil {
			return nil, classify(e)
		}
		all = append(all, r.GetSecrets()...)
		page = r.GetNextPageToken()
		if page == "" {
			s.indexMu.Lock()
			clear(s.index)
			for _, v := range all {
				s.index[v.Name] = v.Id
			}
			s.indexMu.Unlock()
			return all, nil
		}
		if seen[page] {
			return nil, kryptos.ErrMalformed
		}
		seen[page] = true
	}
}
func (s *Storage[T]) find(ctx context.Context, key string) (*pb.Secret, error) {
	s.indexMu.Lock()
	id := s.index[key]
	s.indexMu.Unlock()
	if id != "" {
		meta, err := s.ss.Get(ctx, &pb.GetSecretRequest{SecretId: id})
		if err == nil && meta.GetName() == key {
			if !s.matches(meta) {
				return nil, kryptos.ErrConflict
			}
			return meta, nil
		}
		if err != nil && status.Code(err) != codes.NotFound {
			return nil, classify(err)
		}
		s.indexMu.Lock()
		delete(s.index, key)
		s.indexMu.Unlock()
	}
	all, e := s.secrets(ctx)
	if e != nil {
		return nil, e
	}
	for _, v := range all {
		if v.Name == key {
			if !s.matches(v) {
				return nil, kryptos.ErrConflict
			}
			meta, err := s.ss.Get(ctx, &pb.GetSecretRequest{SecretId: v.Id})
			if err != nil {
				return nil, classify(err)
			}
			if !s.matches(meta) {
				return nil, kryptos.ErrConflict
			}
			return meta, nil
		}
	}
	return nil, kryptos.ErrNotFound
}
