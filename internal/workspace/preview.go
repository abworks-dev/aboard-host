package workspace

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

var (
	ErrPreview  = errors.New("workspace preview input is invalid")
	ErrBaseline = errors.New("workspace preview baseline is no longer current")
)

// A fresh immutable candidate is allocated for EVERY preview, including changed
// text/base. Version 1 is local to that candidate, not a mutable global preview.
const CandidateLifetime = 15 * time.Minute

type Candidate struct {
	WorkspaceID, CandidateID, PrincipalID, ResourceID string
	Kind                                              string
	BaseRevision                                      int64
	BaseDigest                                        *string
	Title, Body, ContentDigest, CandidateDigest       string
	PreviewVersion                                    int64
	PreviewEpoch                                      string
	ExpiresAt                                         time.Time
}

type PreviewInput struct {
	Epoch, Kind, ResourceID string
	BaseRevision            int64
	BaseDigest              *string
	Title, Body             string
}

type Preview struct {
	Candidate Candidate
	Before    *TextRecord
}

// PreviewFacts persists only non-formal candidates on the retained formal owner.
// It must recheck the baseline atomically with persistence, and report uncertain
// physical/persistence completion as ErrOpenUncertain without releasing leases.
// It is not a public filesystem, SQL, authorization or admission capability.
type PreviewFacts interface {
	PutPreview(context.Context, Scope, Candidate) error
}

func (in PreviewInput) validate() error {
	var resource *protocol.UUID
	if in.ResourceID != "" {
		v := protocol.UUID(in.ResourceID)
		resource = &v
	}
	var base *protocol.Digest
	if in.BaseDigest != nil {
		v := protocol.Digest(*in.BaseDigest)
		base = &v
	}
	// Reuse the one generated structure/conditional/text validation authority.
	// In particular Go's JSON encoder must not silently replace invalid UTF-8.
	_, err := (protocol.PreviewRecord{OpenEpoch: protocol.UUID(in.Epoch), Kind: protocol.PreviewRecordKind(in.Kind), ResourceId: resource, BaseRevision: protocol.Revision(strconv.FormatInt(in.BaseRevision, 10)), BaseDigest: base, Title: protocol.Title(in.Title), Body: protocol.Body(in.Body)}).MarshalJSON()
	if err != nil {
		return ErrPreview
	}
	return nil
}

func (o *OpenOwner) PreviewRecord(ctx context.Context, scope Scope, principal string, authorize func(context.Context) error, in PreviewInput) (Preview, error) {
	var out Preview
	err := o.withRead(ctx, scope, authorize, func(f TextFacts) error {
		if err := in.validate(); err != nil {
			return err
		}
		if !recordID(principal) {
			return ErrPreview
		}
		if in.Epoch != o.view.Epoch {
			return ErrStaleHandle
		}
		store, ok := f.(PreviewFacts)
		if !ok {
			return ErrPreview
		}
		snapshot, err := f.Snapshot(ctx, scope)
		if err != nil {
			return err
		}
		if snapshot.Revision != in.BaseRevision {
			return ErrBaseline
		}
		resource := in.ResourceID
		if in.Kind == "create-text" {
			resource, err = newEpoch()
			if err != nil {
				return err
			}
		} else {
			before, err := f.Current(ctx, scope, resource)
			if err != nil {
				return err
			}
			if in.BaseDigest == nil || before.ContentDigest != *in.BaseDigest {
				return ErrBaseline
			}
			out.Before = &before
		}
		id, err := newEpoch()
		if err != nil {
			return err
		}
		content, err := digest.Content(digest.ContentInput{WorkspaceID: protocol.UUID(scope.WorkspaceID), ResourceID: protocol.UUID(resource), Title: protocol.Title(in.Title), Body: protocol.Body(in.Body)})
		if err != nil {
			return ErrPreview
		}
		var base *protocol.Digest
		var baseString *string
		if in.BaseDigest != nil {
			v := protocol.Digest(*in.BaseDigest)
			base = &v
			s := string(v)
			baseString = &s
		}
		hash, err := digest.Candidate(digest.CandidateInput{WorkspaceID: protocol.UUID(scope.WorkspaceID), PrincipalID: protocol.UUID(principal), CandidateID: protocol.UUID(id), ResourceID: protocol.UUID(resource), Kind: protocol.PreviewRecordKind(in.Kind), BaseRevision: protocol.Revision(strconv.FormatInt(in.BaseRevision, 10)), BaseDigest: base, ContentDigest: content, PreviewVersion: "1"})
		if err != nil {
			return ErrPreview
		}
		out.Candidate = Candidate{WorkspaceID: scope.WorkspaceID, CandidateID: id, PrincipalID: principal, ResourceID: resource, Kind: in.Kind, BaseRevision: in.BaseRevision, BaseDigest: baseString, Title: in.Title, Body: in.Body, ContentDigest: string(content), CandidateDigest: string(hash), PreviewVersion: 1, PreviewEpoch: in.Epoch, ExpiresAt: time.Now().UTC().Add(CandidateLifetime)}
		if err = authorize(ctx); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = o.queue.beforeMutation(); err != nil {
			return err
		}
		if err = store.PutPreview(ctx, scope, out.Candidate); err != nil {
			if errors.Is(err, ErrOpenUncertain) {
				// withRead owns the active ticket; unknown persistence retains its capacity.
				o.queue.mu.Lock()
				o.queue.active.hold = "preview:" + id
				o.queue.mu.Unlock()
				o.view.NeedsInvestigation = true
				return errors.Join(err, o.lifecycle.transition(scope, LifecycleUnknown, LifecycleOpen))
			}
			return err
		}
		return nil
	})
	if err != nil {
		return Preview{}, err
	}
	return out, nil
}
