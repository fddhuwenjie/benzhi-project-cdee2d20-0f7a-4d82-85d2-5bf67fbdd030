package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

const maxRequestBytes = 1 << 20

type writeMeta struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	ActorID          string `json:"actor_id"`
}

func (m writeMeta) commandMeta() mission.CommandMeta {
	return mission.CommandMeta{RequestID: strings.TrimSpace(m.RequestID), ExpectedRevision: m.ExpectedRevision, ActorID: strings.TrimSpace(m.ActorID)}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return mission.Invalid("Content-Type", "必须为 application/json")
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return mission.Invalid("Content-Type", "必须为 application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxBytes *http.MaxBytesError
		var syntax *json.SyntaxError
		switch {
		case errors.As(err, &maxBytes):
			return mission.Invalid("body", "请求体超过 1 MiB")
		case errors.As(err, &syntax):
			return mission.Invalid("body", fmt.Sprintf("JSON 在偏移 %d 处格式错误", syntax.Offset))
		case errors.Is(err, io.EOF):
			return mission.Invalid("body", "请求体不能为空")
		default:
			return mission.Invalid("body", err.Error())
		}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return mission.Invalid("body", "只能包含一个 JSON 对象")
	}
	return nil
}
