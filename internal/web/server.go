package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"

	"hello-tsnet/internal/config"
	"hello-tsnet/internal/rules"
	webassets "hello-tsnet/web"
)

const maxRequestBody = 64 << 10

const (
	StatePending = "pending"
	StateReady   = "ready"
	StateError   = "error"
)

type Status struct {
	Hostname string `json:"hostname,omitempty"`
	State   string `json:"state"`
	AuthURL string `json:"authUrl,omitempty"`
	Error   string `json:"error,omitempty"`
}

type RuleService interface {
	List() []config.Rule
	Add(name string, localPort int, target string, protocol config.Protocol, direction config.Direction) (config.Rule, error)
	Update(id, name string, localPort int, target string, protocol config.Protocol, direction config.Direction) (config.Rule, error)
	Delete(id string) error
}

type ruleInput struct {
	Name      string `json:"name"`
	LocalPort int    `json:"localPort"`
	Target    string `json:"target"`
	Protocol config.Protocol `json:"protocol"`
	Direction config.Direction `json:"direction"`
}

func NewHandler(authority string, service RuleService, status func() Status, devices DeviceReader) http.Handler {
	_, port, err := net.SplitHostPort(authority)
	if err != nil {
		panic(fmt.Sprintf("invalid web authority %q: %v", authority, err))
	}
	allowedAuthorities := map[string]struct{}{
		net.JoinHostPort("127.0.0.1", port): {},
		net.JoinHostPort("localhost", port): {},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/devices", devicesHandler(devices))
	mux.HandleFunc("OPTIONS /api/{rest...}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("GET /api/rules", func(w http.ResponseWriter, _ *http.Request) {
		items := service.List()
		if items == nil {
			items = []config.Rule{}
		}
		writeJSON(w, http.StatusOK, items)
	})
	mux.HandleFunc("POST /api/rules", func(w http.ResponseWriter, request *http.Request) {
		if !allowWrite(w, request, allowedAuthorities) {
			return
		}
		input, ok := decodeRuleInput(w, request)
		if !ok {
			return
		}
		rule, err := service.Add(input.Name, input.LocalPort, input.Target, input.Protocol, input.Direction)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	})
	mux.HandleFunc("PUT /api/rules/{id}", func(w http.ResponseWriter, request *http.Request) {
		if !allowWrite(w, request, allowedAuthorities) {
			return
		}
		input, ok := decodeRuleInput(w, request)
		if !ok {
			return
		}
		rule, err := service.Update(request.PathValue("id"), input.Name, input.LocalPort, input.Target, input.Protocol, input.Direction)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rule)
	})
	mux.HandleFunc("DELETE /api/rules/{id}", func(w http.ResponseWriter, request *http.Request) {
		if !allowWrite(w, request, allowedAuthorities) {
			return
		}
		if err := service.Delete(request.PathValue("id")); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		current := status()
		if current.State == StateReady {
			current.AuthURL = ""
		}
		writeJSON(w, http.StatusOK, current)
	})
	mux.Handle("/", http.FileServer(http.FS(webassets.Files)))

	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if _, allowed := allowedAuthorities[request.Host]; !allowed {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, request)
	})
}

func allowWrite(w http.ResponseWriter, request *http.Request, allowed map[string]struct{}) bool {
	if request.Method == http.MethodOptions {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}
	origin := request.Header.Get("Origin")
	validOrigin := false
	for authority := range allowed {
		if origin == "http://"+authority {
			validOrigin = true
			break
		}
	}
	if !validOrigin {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return false
	}
	if request.Method != http.MethodDelete {
		mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return false
		}
	}
	return true
}

func decodeRuleInput(w http.ResponseWriter, request *http.Request) (ruleInput, bool) {
	request.Body = http.MaxBytesReader(w, request.Body, maxRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input ruleInput
	if err := decoder.Decode(&input); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, errors.New("request body is too large"))
		} else {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		}
		return ruleInput{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, errors.New("request body must contain one JSON object"))
		return ruleInput{}, false
	}
	return input, true
}

func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, rules.ErrInvalid) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, rules.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusConflict, err)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(err.Error())})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
