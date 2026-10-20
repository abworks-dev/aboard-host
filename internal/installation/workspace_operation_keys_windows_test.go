//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestWorkspaceOperationKeysSeparateKindsAndScopes(t *testing.T) {
	for _, order := range []string{"save-first", "close-first", "fresh-owner"} {
		t.Run(order, func(t *testing.T) {
			f, creator, operator, token, request := creationFixture(t)
			ctx := context.Background()
			a, err := creator.CreateWorkspace(ctx, operator, request)
			if err != nil {
				t.Fatal(err)
			}
			second := request
			second.OperationID = authSameResource
			second.RootLocator = filepath.Join(f.allowed, "key-scope-b")
			b, err := creator.CreateWorkspace(ctx, operator, second)
			if err != nil {
				t.Fatal(err)
			}
			opener, err := NewWorkspaceOpener(f.auth)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := opener.ShutdownWorkspaces(ctx); err != nil {
					t.Error(err)
				}
			}()
			auth := func(ws, action string) *Authorization {
				t.Helper()
				value, err := f.auth.Authorize(ctx, token, ws, action)
				if err != nil {
					t.Fatal(err)
				}
				return value
			}
			openA, writeA, readA, closeA := auth(a.WorkspaceID, "workspace.open"), auth(a.WorkspaceID, "workspace.write"), auth(a.WorkspaceID, "workspace.read"), auth(a.WorkspaceID, "workspace.close")
			openB, closeB := auth(b.WorkspaceID, "workspace.open"), auth(b.WorkspaceID, "workspace.close")
			av, err := opener.OpenWorkspace(ctx, openA, 1)
			if err != nil {
				t.Fatal(err)
			}
			bv, err := opener.OpenWorkspace(ctx, openB, 1)
			if err != nil {
				t.Fatal(err)
			}
			preview := func(epoch string) workspace.Preview {
				t.Helper()
				p, err := opener.PreviewRecord(ctx, writeA, workspace.PreviewInput{Epoch: epoch, Kind: "create-text", Title: "shared key", Body: "exact original bytes"})
				if err != nil {
					t.Fatal(err)
				}
				return p
			}
			const key = authSameResource
			if order == "close-first" {
				if _, err := opener.CloseWorkspace(ctx, closeA, key, av.Epoch); err != nil {
					t.Fatal(err)
				}
				if v, err := opener.WaitClose(ctx, closeA, key); err != nil || !v.Closed {
					t.Fatal(v, err)
				}
				av, err = opener.OpenWorkspace(ctx, openA, 1)
				if err != nil {
					t.Fatal(err)
				}
				if result, err := opener.CommitRecord(ctx, writeA, commitPreviewInput(key, preview(av.Epoch))); !errors.Is(err, workspace.ErrSaveKeyReuse) || result.Receipt != nil {
					t.Fatal("close key admitted as save", result, err)
				}
				if snapshot, err := opener.ReadSnapshot(ctx, readA); err != nil || snapshot.Revision != 0 {
					t.Fatal(snapshot, err)
				}
				// The same key is independently valid in B, not installation-global.
				if _, err := opener.CloseWorkspace(ctx, closeB, key, bv.Epoch); err != nil {
					t.Fatal("cross-scope key collision", err)
				}
				return
			}
			p := preview(av.Epoch)
			input := commitPreviewInput(key, p)
			saved, err := opener.CommitRecord(ctx, writeA, input)
			if err != nil || saved.Status != workspace.SaveConfirmed {
				t.Fatal(saved, err)
			}
			if order == "fresh-owner" {
				if err := opener.ShutdownWorkspaces(ctx); err != nil {
					t.Fatal(err)
				}
				if err := f.store.Close(); err != nil {
					t.Fatal(err)
				}
				fresh, err := Open(ctx, f.install)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := fresh.Close(); err != nil {
						t.Error(err)
					}
				}()
				freshAuth, err := NewAuthenticator(fresh, []string{f.allowed})
				if err != nil {
					t.Fatal(err)
				}
				_, freshToken := authOperator(t, authFixture{store: fresh, auth: freshAuth, root: f.root, allowed: f.allowed, install: f.install})
				current, err := NewWorkspaceOpener(freshAuth)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := current.ShutdownWorkspaces(ctx); err != nil {
						t.Error(err)
					}
				}()
				freshAction := func(action string) *Authorization {
					t.Helper()
					a, err := freshAuth.Authorize(ctx, freshToken, a.WorkspaceID, action)
					if err != nil {
						t.Fatal(err)
					}
					return a
				}
				v, err := current.OpenWorkspace(ctx, freshAction("workspace.open"), 1)
				if err != nil || v.Epoch == av.Epoch {
					t.Fatal(v, err)
				}
				if got, err := current.CloseWorkspace(ctx, freshAction("workspace.close"), key, v.Epoch); !errors.Is(err, workspace.ErrSaveKeyReuse) || got.OperationID != "" {
					t.Fatal("fresh owner lost durable save kind", got, err)
				}
				original, err := current.CommitRecord(ctx, freshAction("workspace.write"), input)
				if err != nil || original.Receipt == nil || *original.Receipt != *saved.Receipt {
					t.Fatal("original receipt changed", original, err)
				}
				return
			}
			if got, err := opener.CloseWorkspace(ctx, closeA, key, av.Epoch); !errors.Is(err, workspace.ErrSaveKeyReuse) || got.OperationID != "" {
				t.Fatal("save key admitted as close", got, err)
			}
			if snapshot, err := opener.ReadSnapshot(ctx, readA); err != nil || snapshot.Revision != 1 {
				t.Fatal("rejected collision closed A", snapshot, err)
			}
			if _, err := opener.CloseWorkspace(ctx, closeB, key, bv.Epoch); err != nil {
				t.Fatal("cross-scope key collision", err)
			}
		})
	}
}
