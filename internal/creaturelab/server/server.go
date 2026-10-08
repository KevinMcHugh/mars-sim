package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	apigen "github.com/kevinmchugh/mars-sim/internal/creaturelab/api/gen"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server/export"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server/species"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server/sprites"
)

// BaseURL is where the API is mounted: the spec's servers entry.
const BaseURL = "/api"

// Store is everything the endpoints persist through. *creaturelab.Store
// satisfies it; each endpoint narrows it to its own interface.
type Store interface {
	db.Querier
	AcceptCandidate(ctx context.Context, id string) (db.SpriteCandidate, error)
}

// Server implements apigen.StrictServerInterface by composing per-resource
// endpoints.
type Server struct {
	store Store
	links creaturelab.Links
}

// New builds the API on store, with links for the URLs in its responses.
func New(store Store, links creaturelab.Links) *Server {
	return &Server{store: store, links: links}
}

// Compile-time assertion.
var _ apigen.StrictServerInterface = (*Server)(nil)

// -----------------------------------------------------------------------------
// Mounting
// -----------------------------------------------------------------------------

// Mount attaches the API to r under BaseURL. requireAuth guards every
// operation except those the spec marks public with `security: []`, so the
// spec, not the router, decides who may call what.
func (s *Server) Mount(r chi.Router, requireAuth func(http.Handler) http.Handler) {
	public := PublicOperations()
	guard := func(next http.Handler) http.Handler {
		protected := requireAuth(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pattern := strings.TrimPrefix(chi.RouteContext(r.Context()).RoutePattern(), BaseURL)
			if public[r.Method+" "+pattern] {
				// Anyone may read these, so any site may too: Scum Lab's
				// Species Catalog fetches them from GitHub Pages. No
				// credentials are allowed cross-site, so this opens nothing
				// an anonymous caller could not already get.
				w.Header().Set("Access-Control-Allow-Origin", "*")
				next.ServeHTTP(w, r)
				return
			}
			protected.ServeHTTP(w, r)
		})
	}
	strict := apigen.NewStrictHandlerWithOptions(s, nil, apigen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: internalError,
	})
	r.Group(func(r chi.Router) {
		r.Use(limitBody)
		apigen.HandlerWithOptions(strict, apigen.ChiServerOptions{
			BaseURL:          BaseURL,
			BaseRouter:       r,
			Middlewares:      []apigen.MiddlewareFunc{guard},
			ErrorHandlerFunc: badRequest,
		})
	})
}

// PublicOperations is the set of operations ("GET /species/{speciesId}")
// whose spec entry overrides the global security with an empty list. An
// operation that does not mention security inherits the global requirement,
// so a new endpoint is private until the spec says otherwise.
func PublicOperations() map[string]bool {
	spec, err := apigen.GetSwagger()
	if err != nil {
		// The spec is embedded at build time, so this is a broken build.
		panic("creature-lab: embedded OpenAPI spec: " + err.Error())
	}
	public := map[string]bool{}
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Security != nil && len(*op.Security) == 0 {
				public[method+" "+path] = true
			}
		}
	}
	return public
}

// limitBody caps a request body: the largest is one SVG.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 2*creaturelab.MaxSVGBytes)
		next.ServeHTTP(w, r)
	})
}

func badRequest(w http.ResponseWriter, _ *http.Request, err error) {
	writeError(w, http.StatusBadRequest, err.Error())
}

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("api", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apigen.Error{Error: msg})
}

// -----------------------------------------------------------------------------
// Errors
// -----------------------------------------------------------------------------

func isNotFound(err error) bool { return errors.Is(err, creaturelab.ErrNotFound) }

// isUnprocessable is a request that parsed but cannot be done: a form the
// species does not have, an SVG that was refused, or a field out of bounds.
func isUnprocessable(err error) bool {
	return errors.Is(err, creaturelab.ErrNoSuchForm) || errors.Is(err, creaturelab.ErrBadSVG) || errors.Is(err, creaturelab.ErrBadInput)
}

func notFound(err error) apigen.NotFoundJSONResponse {
	return apigen.NotFoundJSONResponse{Error: err.Error()}
}

func unprocessable(err error) apigen.UnprocessableJSONResponse {
	return apigen.UnprocessableJSONResponse{Error: err.Error()}
}

// -----------------------------------------------------------------------------
// Species
// -----------------------------------------------------------------------------

func (s *Server) PreviewSpecies(ctx context.Context, req apigen.PreviewSpeciesRequestObject) (apigen.PreviewSpeciesResponseObject, error) {
	return Run(ctx, species.PreviewEndpoint{}, req)
}

func (s *Server) ListSpecies(ctx context.Context, req apigen.ListSpeciesRequestObject) (apigen.ListSpeciesResponseObject, error) {
	return Run(ctx, species.ListEndpoint{Store: s.store, Links: s.links}, req)
}

func (s *Server) GetSpecies(ctx context.Context, req apigen.GetSpeciesRequestObject) (apigen.GetSpeciesResponseObject, error) {
	resp, err := Run(ctx, species.GetEndpoint{Store: s.store, Links: s.links}, req)
	if isNotFound(err) {
		return apigen.GetSpecies404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	}
	return resp, err
}

func (s *Server) CreateSpecies(ctx context.Context, req apigen.CreateSpeciesRequestObject) (apigen.CreateSpeciesResponseObject, error) {
	if req.Body == nil {
		return apigen.CreateSpecies400JSONResponse{BadRequestJSONResponse: apigen.BadRequestJSONResponse{Error: "body required"}}, nil
	}
	return Run(ctx, species.CreateEndpoint{Store: s.store, Links: s.links}, req)
}

func (s *Server) UpdateSpecies(ctx context.Context, req apigen.UpdateSpeciesRequestObject) (apigen.UpdateSpeciesResponseObject, error) {
	if req.Body == nil {
		return apigen.UpdateSpecies400JSONResponse{BadRequestJSONResponse: apigen.BadRequestJSONResponse{Error: "body required"}}, nil
	}
	resp, err := Run(ctx, species.UpdateEndpoint{Store: s.store, Links: s.links}, req)
	switch {
	case isNotFound(err):
		return apigen.UpdateSpecies404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	case isUnprocessable(err):
		return apigen.UpdateSpecies422JSONResponse{UnprocessableJSONResponse: unprocessable(err)}, nil
	}
	return resp, err
}

func (s *Server) DeleteSpecies(ctx context.Context, req apigen.DeleteSpeciesRequestObject) (apigen.DeleteSpeciesResponseObject, error) {
	resp, err := Run(ctx, species.DeleteEndpoint{Store: s.store}, req)
	if isNotFound(err) {
		return apigen.DeleteSpecies404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	}
	return resp, err
}

func (s *Server) ExportCatalog(ctx context.Context, req apigen.ExportCatalogRequestObject) (apigen.ExportCatalogResponseObject, error) {
	return Run(ctx, export.Endpoint{Store: s.store}, req)
}

// -----------------------------------------------------------------------------
// Sprites
// -----------------------------------------------------------------------------

func (s *Server) GetSpriteBrief(ctx context.Context, req apigen.GetSpriteBriefRequestObject) (apigen.GetSpriteBriefResponseObject, error) {
	resp, err := Run(ctx, sprites.BriefEndpoint{Store: s.store}, req)
	switch {
	case isNotFound(err):
		return apigen.GetSpriteBrief404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	case isUnprocessable(err):
		return apigen.GetSpriteBrief422JSONResponse{UnprocessableJSONResponse: unprocessable(err)}, nil
	}
	return resp, err
}

func (s *Server) ListSpriteCandidates(ctx context.Context, req apigen.ListSpriteCandidatesRequestObject) (apigen.ListSpriteCandidatesResponseObject, error) {
	resp, err := Run(ctx, sprites.ListEndpoint{Store: s.store, Links: s.links}, req)
	if isNotFound(err) {
		return apigen.ListSpriteCandidates404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	}
	return resp, err
}

func (s *Server) SubmitSpriteCandidate(ctx context.Context, req apigen.SubmitSpriteCandidateRequestObject) (apigen.SubmitSpriteCandidateResponseObject, error) {
	if req.Body == nil {
		return apigen.SubmitSpriteCandidate400JSONResponse{BadRequestJSONResponse: apigen.BadRequestJSONResponse{Error: "body required"}}, nil
	}
	resp, err := Run(ctx, sprites.SubmitEndpoint{Store: s.store, Links: s.links}, req)
	switch {
	case isNotFound(err):
		return apigen.SubmitSpriteCandidate404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	case isUnprocessable(err):
		return apigen.SubmitSpriteCandidate422JSONResponse{UnprocessableJSONResponse: unprocessable(err)}, nil
	}
	return resp, err
}

func (s *Server) GetSpriteCandidate(ctx context.Context, req apigen.GetSpriteCandidateRequestObject) (apigen.GetSpriteCandidateResponseObject, error) {
	resp, err := Run(ctx, sprites.GetEndpoint{Store: s.store, Links: s.links}, req)
	if isNotFound(err) {
		return apigen.GetSpriteCandidate404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	}
	return resp, err
}

func (s *Server) AcceptSpriteCandidate(ctx context.Context, req apigen.AcceptSpriteCandidateRequestObject) (apigen.AcceptSpriteCandidateResponseObject, error) {
	resp, err := Run(ctx, sprites.AcceptEndpoint{Store: s.store, Links: s.links}, req)
	if isNotFound(err) {
		return apigen.AcceptSpriteCandidate404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	}
	return resp, err
}

func (s *Server) DeleteSpriteCandidate(ctx context.Context, req apigen.DeleteSpriteCandidateRequestObject) (apigen.DeleteSpriteCandidateResponseObject, error) {
	resp, err := Run(ctx, sprites.DeleteEndpoint{Store: s.store}, req)
	if isNotFound(err) {
		return apigen.DeleteSpriteCandidate404JSONResponse{NotFoundJSONResponse: notFound(err)}, nil
	}
	return resp, err
}
