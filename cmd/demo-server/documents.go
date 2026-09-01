package main

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// documentsCollection is the MongoDB collection backing /documents.
const documentsCollection = "documents"

// document is a stored document. The BSON `_id` is rendered as `id` in JSON,
// which is what the API has always exposed.
type document struct {
	ID        bson.ObjectID `bson:"_id,omitempty" json:"id"`
	Title     string        `bson:"title"        json:"title"`
	Content   string        `bson:"content"      json:"content"`
	Tags      []string      `bson:"tags"         json:"tags"`
	CreatedAt time.Time     `bson:"createdAt"    json:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt"    json:"updatedAt"`
}

// docs returns the documents collection, or nil when MongoDB is unavailable.
func (s *services) docs() *mongo.Collection {
	if s.mongoDB == nil {
		return nil
	}
	return s.mongoDB.Collection(documentsCollection)
}

func (s *services) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	coll := s.docs()
	if coll == nil {
		writeError(w, http.StatusInternalServerError, "Failed to create document")
		return
	}

	var req struct {
		Title   string   `json:"title"`
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Title == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "title and content are required")
		return
	}

	now := time.Now().UTC()
	doc := document{
		Title:   req.Title,
		Content: req.Content,
		// An absent tags field stores and returns [] rather than null, so
		// filtering and length assertions behave the same either way.
		Tags:      orEmpty(req.Tags),
		CreatedAt: now,
		UpdatedAt: now,
	}

	res, err := coll.InsertOne(r.Context(), doc)
	if err != nil {
		log.Printf("error creating document: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to create document")
		return
	}
	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		doc.ID = oid
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (s *services) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	coll := s.docs()
	if coll == nil {
		writeError(w, http.StatusInternalServerError, "Failed to list documents")
		return
	}

	filter := bson.M{}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		filter["tags"] = tag
	}
	if title := r.URL.Query().Get("title"); title != "" {
		// Case-insensitive substring match, as the previous $regex/$options
		// filter did. The pattern is quoted so a title containing regex
		// metacharacters matches literally.
		filter["title"] = bson.M{"$regex": regexp.QuoteMeta(title), "$options": "i"}
	}

	cursor, err := coll.Find(r.Context(), filter)
	if err != nil {
		log.Printf("error listing documents: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to list documents")
		return
	}
	defer cursor.Close(context.Background())

	documents := []document{}
	if err := cursor.All(r.Context(), &documents); err != nil {
		log.Printf("error listing documents: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to list documents")
		return
	}
	for i := range documents {
		documents[i].Tags = orEmpty(documents[i].Tags)
	}
	writeJSON(w, http.StatusOK, documents)
}

func (s *services) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	coll := s.docs()
	if coll == nil {
		writeError(w, http.StatusInternalServerError, "Failed to get document")
		return
	}

	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		// A malformed id cannot name a document; that is a 404, not a 500.
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}

	var doc document
	err = coll.FindOne(r.Context(), bson.M{"_id": id}).Decode(&doc)
	switch {
	case err == mongo.ErrNoDocuments:
		writeError(w, http.StatusNotFound, "Document not found")
	case err != nil:
		log.Printf("error getting document: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get document")
	default:
		doc.Tags = orEmpty(doc.Tags)
		writeJSON(w, http.StatusOK, doc)
	}
}

func (s *services) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	coll := s.docs()
	if coll == nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete document")
		return
	}

	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}

	res, err := coll.DeleteOne(r.Context(), bson.M{"_id": id})
	if err != nil {
		log.Printf("error deleting document: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to delete document")
		return
	}
	if res.DeletedCount == 0 {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *services) handleDeleteDocuments(w http.ResponseWriter, r *http.Request) {
	coll := s.docs()
	if coll == nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete documents")
		return
	}

	filter := bson.M{}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		filter["tags"] = tag
	}
	if title := r.URL.Query().Get("title"); title != "" {
		filter["title"] = title
	}
	// Refuse an unfiltered bulk delete: without this, a missing query parameter
	// would silently empty the collection.
	if len(filter) == 0 {
		writeError(w, http.StatusBadRequest, "Filter required (tag or title)")
		return
	}

	res, err := coll.DeleteMany(r.Context(), filter)
	if err != nil {
		log.Printf("error deleting documents: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to delete documents")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deletedCount": res.DeletedCount})
}

// orEmpty replaces a nil slice with an empty one so it serialises as [].
func orEmpty(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
