package httpapi

import (
	"net/http"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

type Server struct {
	missions *mission.Service
	audit    *audit.Service
	handler  http.Handler
}

func New(missions *mission.Service, auditService *audit.Service) *Server {
	s := &Server{missions: missions, audit: auditService}
	s.handler = s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.HealthHandler)
	mux.HandleFunc("POST /api/v1/dive-missions", s.CreateMissionHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/schedule-preflight", s.SchedulePreflightHandler)
	mux.HandleFunc("GET /api/v1/dive-missions", s.ListMissionsHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}", s.GetMissionHandler)
	mux.HandleFunc("PATCH /api/v1/dive-missions/{id}", s.ReviseDraftHandler)
	mux.HandleFunc("PUT /api/v1/dive-missions/{id}", s.ReviseDraftHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/history", s.GetHistoryHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/audit-events", s.GetAuditEventsHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/release-preview", s.ReleasePreviewHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/release-checklist", s.ReleasePreviewHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/risks", s.SubmitRisksHandler)
	mux.HandleFunc("PUT /api/v1/dive-missions/{id}/risks", s.ReassessRisksHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/risks/reassess", s.ReassessRisksHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/risks/mitigations/{action_code}/complete", s.CompleteMitigationHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/risks/mitigations/complete-batch", s.CompleteMitigationBatchHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/risks/mitigations/batch/complete", s.CompleteMitigationBatchHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/life-support-plan", s.SubmitPlanHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/life-support-plan", s.GetPlanHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/life-support-plans/{plan_id}", s.GetPlanHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/life-support-review", s.ReviewPlanHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/equipment-verifications", s.VerifyEquipmentHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/drills", s.RecordDrillHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/remediations", s.RecordRemediationHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/retests", s.RecordRetestHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/release", s.ReleaseMissionHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/archive", s.ArchiveMissionHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/archive-export", s.ArchiveExportHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/archive", s.ArchiveExportHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/archive/evidence", s.ArchiveEvidenceHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/archive-integrity", s.ArchiveIntegrityHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/template-preview", s.TemplatePreviewHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/template-preview", s.TemplatePreviewHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/template-preview", s.TemplatePreviewHandler)
	mux.HandleFunc("POST /api/v1/dive-missions/{id}/template-preview", s.TemplatePreviewHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/risks/mitigations", s.MitigationQueryHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/risk-mitigations", s.MitigationQueryHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/equipment-evidence", s.EquipmentEvidenceHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/equipment-verifications", s.EquipmentEvidenceHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/equipment/evidence", s.EquipmentEvidenceHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/drill-remediation-review", s.RemediationReviewHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/remediation-review", s.RemediationReviewHandler)
	mux.HandleFunc("GET /api/v1/dive-missions/{id}/drills/remediation-review", s.RemediationReviewHandler)
	return requestMiddleware(mux)
}

func requestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		started := time.Now()
		next.ServeHTTP(w, r)
		_ = started
	})
}
