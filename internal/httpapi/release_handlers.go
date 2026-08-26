package httpapi

import (
	"net/http"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

func (s *Server) ArchiveExportHandler(w http.ResponseWriter, r *http.Request) {
	after, err := parseNonNegative(r.URL.Query().Get("after"), 0)
	if err != nil {
		writeError(w, mission.Invalid("after", "必须为非负整数"), "")
		return
	}
	limit, err := parseNonNegative(r.URL.Query().Get("limit"), 50)
	if err != nil || limit == 0 || limit > 200 {
		writeError(w, mission.Invalid("limit", "必须在 1 到 200 之间"), "")
		return
	}
	result, err := s.missions.ExportArchive(r.Context(), r.PathValue("id"), int64(after), limit, r.URL.Query().Get("event_type"))
	if err != nil {
		writeError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ArchiveEvidenceHandler(w http.ResponseWriter, r *http.Request) {
	cursor, err := parseNonNegative(r.URL.Query().Get("cursor"), 0)
	if err != nil {
		writeError(w, mission.Invalid("cursor", "必须为非负整数"), "")
		return
	}
	limit, err := parseNonNegative(r.URL.Query().Get("limit"), 50)
	if err != nil || limit == 0 || limit > 200 {
		writeError(w, mission.Invalid("limit", "必须在 1 到 200 之间"), "")
		return
	}
	result, err := s.missions.LocateArchiveEvidence(r.Context(), r.PathValue("id"), mission.ArchiveEvidenceFilter{GateCode: r.URL.Query().Get("gate_code"), RecordID: r.URL.Query().Get("record_id"), ActorID: r.URL.Query().Get("actor_id"), EventType: r.URL.Query().Get("event_type"), Cursor: int64(cursor), Limit: limit})
	if err != nil {
		writeError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ReleasePreviewHandler(w http.ResponseWriter, r *http.Request) {
	p, err := s.missions.PreviewRelease(r.Context(), r.PathValue("id"), r.URL.Query().Get("supervisor_id"))
	if err != nil {
		writeError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) ReleaseMissionHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		mission.ReleaseInput
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	request.ReleaseInput.Meta = request.writeMeta.commandMeta()
	result, replay, err := s.missions.Release(r.Context(), r.PathValue("id"), request.ReleaseInput)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}

func (s *Server) ArchiveMissionHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		writeMeta
		mission.ArchiveInput
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	request.ArchiveInput.Meta = request.writeMeta.commandMeta()
	result, replay, err := s.missions.Archive(r.Context(), r.PathValue("id"), request.ArchiveInput)
	if err != nil {
		writeError(w, err, request.RequestID)
		return
	}
	writeStored(w, result, replay, request.RequestID)
}
