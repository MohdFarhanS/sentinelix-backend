package http

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/MohdFarhanS/sentinelix-backend/pkg/jwt"
)

func NewRouter(
	authHandler *AuthHandler,
	ingestHandler *IngestHandler,
	projectHandler *ProjectHandler,
	issueHandler *IssueHandler,
	alertRuleHandler *AlertRuleHandler,
	monitorHandler *MonitorHandler,
	wsHandler *WSHandler,
	jwtManager *jwt.Manager,
	frontendURL string,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(CORSMiddleware(frontendURL))

	r.Route("/api/v1", func(r chi.Router) {
		// Publik
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/logout", authHandler.Logout)
		r.Post("/ingest/event", ingestHandler.HandleIngestEvent)

		// Protected — butuh JWT via httpOnly cookie
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(jwtManager))

			r.Post("/projects", projectHandler.Create)
			r.Get("/projects", projectHandler.List)
			r.Get("/projects/{projectId}/issues", issueHandler.List)
			r.Get("/issues/{id}", issueHandler.GetByID)
			r.Get("/issues/{id}/events", issueHandler.ListEvents)
			r.Post("/projects/{projectId}/alert-rules", alertRuleHandler.Create)
			r.Get("/projects/{projectId}/alert-rules", alertRuleHandler.List)
			r.Patch("/alert-rules/{id}", alertRuleHandler.Update)
			r.Delete("/alert-rules/{id}", alertRuleHandler.Delete)
			r.Post("/projects/{projectId}/monitors", monitorHandler.Create)
			r.Get("/projects/{projectId}/monitors", monitorHandler.List)
			r.Get("/monitors/{id}", monitorHandler.GetByID)
			r.Patch("/monitors/{id}", monitorHandler.Update)
			r.Delete("/monitors/{id}", monitorHandler.Delete)
			r.Get("/monitors/{id}/checks", monitorHandler.ListChecks)
			r.Get("/ws/projects/{id}", wsHandler.HandleConnection(frontendURL))
		})
	})

	return r
}