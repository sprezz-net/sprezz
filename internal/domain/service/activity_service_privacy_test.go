package service_test

import (
	"context"
	"testing"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"

	"github.com/gojuno/minimock/v3"
)

func TestActivityService_FilterPublicAndAuthorizedQuads(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, portmock.NewRemoteFetcherMock(mc), service.ActivityServiceConfig{})
	ctx := context.Background()

	readerAlice := "https://sprezz.net/alice"

	quadsFixture := []model.Quad{
		// Activity 1: Public Note from Bob
		{GraphID: 101, Subject: "https://remote.com/activity/1", Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", Object: "https://www.w3.org/ns/activitystreams#Create", ObjType: model.NamedNode},
		{GraphID: 101, Subject: "https://remote.com/activity/1", Predicate: "https://www.w3.org/ns/activitystreams#actor", Object: "https://remote.com/bob", ObjType: model.NamedNode},
		{GraphID: 101, Subject: "https://remote.com/activity/1", Predicate: "https://www.w3.org/ns/activitystreams#to", Object: "https://www.w3.org/ns/activitystreams#Public", ObjType: model.NamedNode},
		{GraphID: 101, Subject: "https://remote.com/activity/1", Predicate: "https://www.w3.org/ns/activitystreams#object", Object: "https://remote.com/note/1", ObjType: model.NamedNode},
		{GraphID: 101, Subject: "https://remote.com/note/1", Predicate: "https://www.w3.org/ns/activitystreams#attributedTo", Object: "https://remote.com/bob", ObjType: model.NamedNode},
		{GraphID: 101, Subject: "https://remote.com/note/1", Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", Object: "https://www.w3.org/ns/activitystreams#Note", ObjType: model.NamedNode},
		{GraphID: 101, Subject: "https://remote.com/note/1", Predicate: "https://www.w3.org/ns/activitystreams#content", Object: "Hello World!", ObjType: model.Literal},

		// Activity 2: Private Direct Message to Alice
		{GraphID: 102, Subject: "https://remote.com/activity/2", Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", Object: "https://www.w3.org/ns/activitystreams#Create", ObjType: model.NamedNode},
		{GraphID: 102, Subject: "https://remote.com/activity/2", Predicate: "https://www.w3.org/ns/activitystreams#actor", Object: "https://remote.com/bob", ObjType: model.NamedNode},
		{GraphID: 102, Subject: "https://remote.com/activity/2", Predicate: "https://www.w3.org/ns/activitystreams#to", Object: "https://sprezz.net/alice", ObjType: model.NamedNode},
		{GraphID: 102, Subject: "https://remote.com/activity/2", Predicate: "https://www.w3.org/ns/activitystreams#object", Object: "https://remote.com/note/2", ObjType: model.NamedNode},
		{GraphID: 102, Subject: "https://remote.com/note/2", Predicate: "https://www.w3.org/ns/activitystreams#attributedTo", Object: "https://remote.com/bob", ObjType: model.NamedNode},
		{GraphID: 102, Subject: "https://remote.com/note/2", Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", Object: "https://www.w3.org/ns/activitystreams#Note", ObjType: model.NamedNode},
		{GraphID: 102, Subject: "https://remote.com/note/2", Predicate: "https://www.w3.org/ns/activitystreams#content", Object: "Secret Message!", ObjType: model.Literal},

		// Activity 3: Private Note Addressed to Bob Only
		{GraphID: 103, Subject: "https://sprezz.net/activity/1", Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", Object: "https://www.w3.org/ns/activitystreams#Create", ObjType: model.NamedNode},
		{GraphID: 103, Subject: "https://sprezz.net/activity/1", Predicate: "https://www.w3.org/ns/activitystreams#actor", Object: "https://sprezz.net/alice", ObjType: model.NamedNode},
		{GraphID: 103, Subject: "https://sprezz.net/activity/1", Predicate: "https://www.w3.org/ns/activitystreams#to", Object: "https://remote.com/bob", ObjType: model.NamedNode},
		{GraphID: 103, Subject: "https://sprezz.net/activity/1", Predicate: "https://www.w3.org/ns/activitystreams#object", Object: "https://sprezz.net/note/1", ObjType: model.NamedNode},
		{GraphID: 103, Subject: "https://sprezz.net/note/1", Predicate: "https://www.w3.org/ns/activitystreams#attributedTo", Object: "https://sprezz.net/alice", ObjType: model.NamedNode},
		{GraphID: 103, Subject: "https://sprezz.net/note/1", Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", Object: "https://www.w3.org/ns/activitystreams#Note", ObjType: model.NamedNode},
		{GraphID: 103, Subject: "https://sprezz.net/note/1", Predicate: "https://www.w3.org/ns/activitystreams#content", Object: "For Bob's eyes only!", ObjType: model.Literal},
	}

	tests := []struct {
		name              string
		readerIRI         string
		expectedGraphIDs  map[int64]struct{}
		expectedQuadCount int
	}{
		{
			name:      "Alice Reads: Can see Public and Direct Messages targeted to her",
			readerIRI: readerAlice,
			expectedGraphIDs: map[int64]struct{}{
				101: {},
				102: {},
			},
			expectedQuadCount: 14,
		},
		{
			name:      "Anonymous Reader: Can only see explicitly Public Activities",
			readerIRI: "",
			expectedGraphIDs: map[int64]struct{}{
				101: {},
			},
			expectedQuadCount: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := svc.FilterPublicAndAuthorizedQuads(ctx, tt.readerIRI, quadsFixture)

			if len(filtered) != tt.expectedQuadCount {
				t.Errorf("Filter count incorrect: got %d, want %d", len(filtered), tt.expectedQuadCount)
			}

			for _, q := range filtered {
				if _, allowed := tt.expectedGraphIDs[q.GraphID]; !allowed {
					t.Errorf("Privacy breach: leaked unauthorized GraphID %d in result set", q.GraphID)
				}
			}
		})
	}
}
