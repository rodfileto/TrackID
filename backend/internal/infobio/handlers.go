package infobio

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type authRequest struct {
	Password string `json:"password"`
}

type detailsRequest struct {
	NIF string `json:"nif"`
}

type searchRequest struct {
	RNM            string `json:"rnm"`
	RIN            string `json:"rin"`
	NIF            string `json:"nif"`
	Nome           string `json:"nome"`
	NomePai        string `json:"nome_pai"`
	NomeMae        string `json:"nome_mae"`
	DataNascimento string `json:"data_nascimento"`
}

type authStatusResponse struct {
	Active bool `json:"active"`
}

func AuthStatusHandler(manager *SessionManager) gin.HandlerFunc {
	return func(context *gin.Context) {
		if manager == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "infobio auth is not configured"})
			return
		}
		userID, ok := userIDFromContext(context)
		if !ok {
			context.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}
		active, err := manager.SessionStatus(context.Request.Context(), userID)
		if err != nil {
			context.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		context.JSON(http.StatusOK, authStatusResponse{Active: active})
	}
}

func AuthCreateHandler(manager *SessionManager) gin.HandlerFunc {
	return func(context *gin.Context) {
		if manager == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "infobio auth is not configured"})
			return
		}
		userID, ok := userIDFromContext(context)
		if !ok {
			context.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}
		username := strings.TrimSpace(context.GetString("username"))
		if username == "" {
			context.JSON(http.StatusBadRequest, gin.H{"error": "username claim is required"})
			return
		}
		var request authRequest
		if err := context.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Password) == "" {
			context.JSON(http.StatusBadRequest, gin.H{"error": "Senha e obrigatoria"})
			return
		}
		if err := manager.CreateUserSession(context.Request.Context(), userID, username, request.Password); err != nil {
			context.JSON(http.StatusUnauthorized, gin.H{
				"error":  "Credenciais invalidas ou servico InfoBio indisponivel",
				"detail": err.Error(),
			})
			return
		}
		context.JSON(http.StatusOK, authStatusResponse{Active: true})
	}
}

func AuthDeleteHandler(manager *SessionManager) gin.HandlerFunc {
	return func(context *gin.Context) {
		if manager == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "infobio auth is not configured"})
			return
		}
		userID, ok := userIDFromContext(context)
		if !ok {
			context.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}
		_ = manager.InvalidateUserSession(context.Request.Context(), userID)
		context.JSON(http.StatusOK, authStatusResponse{Active: false})
	}
}

func SearchHandler(service *SearchService, manager *SessionManager) gin.HandlerFunc {
	return func(context *gin.Context) {
		if service == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "infobio search is not configured"})
			return
		}
		cookies, ok := validatedCookies(context, manager)
		if !ok {
			return
		}

		request := searchRequest{
			RNM:            strings.TrimSpace(context.PostForm("rnm")),
			RIN:            strings.TrimSpace(context.PostForm("rin")),
			NIF:            strings.TrimSpace(context.PostForm("nif")),
			Nome:           strings.TrimSpace(context.PostForm("nome")),
			NomePai:        strings.TrimSpace(context.PostForm("nome_pai")),
			NomeMae:        strings.TrimSpace(context.PostForm("nome_mae")),
			DataNascimento: strings.TrimSpace(context.PostForm("data_nascimento")),
		}
		if request.RNM == "" && request.RIN == "" && request.NIF == "" && request.Nome == "" {
			var jsonRequest searchRequest
			if err := context.ShouldBindJSON(&jsonRequest); err == nil {
				request.RNM = strings.TrimSpace(jsonRequest.RNM)
				request.RIN = strings.TrimSpace(jsonRequest.RIN)
				request.NIF = strings.TrimSpace(jsonRequest.NIF)
				request.Nome = strings.TrimSpace(jsonRequest.Nome)
				request.NomePai = strings.TrimSpace(jsonRequest.NomePai)
				request.NomeMae = strings.TrimSpace(jsonRequest.NomeMae)
				request.DataNascimento = strings.TrimSpace(jsonRequest.DataNascimento)
			}
		}

		hook := manager.HookFromCookies(cookies)
		boundService := NewSearchService(service.BaseURL, service.HTTPClient, hook)

		switch {
		case request.RNM != "":
			rinValue := "E" + strings.TrimLeft(request.RNM, "E")
			result, found, err := findOneByField(context, boundService, SearchParams{RIN: rinValue})
			if err != nil {
				context.JSON(http.StatusBadGateway, gin.H{"error": "Erro ao buscar no InfoBio", "detail": err.Error()})
				return
			}
			if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "RNM " + request.RNM + " nao encontrado no InfoBio"})
				return
			}
			context.JSON(http.StatusOK, gin.H{"search_type": "rnm", "rnm": request.RNM, "rin": rinValue, "resultado": result})
		case request.RIN != "":
			result, found, err := findOneByField(context, boundService, SearchParams{RIN: request.RIN})
			if err != nil {
				context.JSON(http.StatusBadGateway, gin.H{"error": "Erro ao buscar no InfoBio", "detail": err.Error()})
				return
			}
			if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "RIN " + request.RIN + " nao encontrado no InfoBio"})
				return
			}
			context.JSON(http.StatusOK, gin.H{"search_type": "rin", "rin": request.RIN, "resultado": result})
		case request.NIF != "":
			result, found, err := findOneByField(context, boundService, SearchParams{NIF: request.NIF})
			if err != nil {
				context.JSON(http.StatusBadGateway, gin.H{"error": "Erro ao buscar no InfoBio", "detail": err.Error()})
				return
			}
			if !found {
				context.JSON(http.StatusNotFound, gin.H{"error": "NIF " + request.NIF + " nao encontrado no InfoBio"})
				return
			}
			context.JSON(http.StatusOK, gin.H{"search_type": "nif", "nif": request.NIF, "resultado": result})
		case request.Nome != "":
			result, err := boundService.Search(context.Request.Context(), SearchParams{Nome: request.Nome, NomePai: request.NomePai, NomeMae: request.NomeMae, DataNascimento: request.DataNascimento})
			if err != nil {
				context.JSON(http.StatusBadGateway, gin.H{"error": "Erro ao buscar no InfoBio", "detail": err.Error()})
				return
			}
			if len(result.Entries) == 0 {
				context.JSON(http.StatusNotFound, gin.H{"error": "Nome \"" + request.Nome + "\" nao encontrado no InfoBio"})
				return
			}
			context.JSON(http.StatusOK, gin.H{"search_type": "name", "nome": request.Nome, "total_resultados": len(result.Entries), "resultados": result.Entries})
		default:
			context.JSON(http.StatusBadRequest, gin.H{"error": "Forneca pelo menos um dos campos: rnm, rin, nif, ou nome"})
		}
	}
}

func DetailsHandler(service *SearchService, manager *SessionManager) gin.HandlerFunc {
	return func(context *gin.Context) {
		if service == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "infobio search is not configured"})
			return
		}
		cookies, ok := validatedCookies(context, manager)
		if !ok {
			return
		}
		var request detailsRequest
		if err := context.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.NIF) == "" {
			context.JSON(http.StatusBadRequest, gin.H{"error": "NIF e obrigatorio"})
			return
		}
		boundService := NewSearchService(service.BaseURL, service.HTTPClient, manager.HookFromCookies(cookies))
		result, err := boundService.Search(context.Request.Context(), SearchParams{NIF: request.NIF})
		if err != nil {
			context.JSON(http.StatusBadGateway, gin.H{"error": "Erro ao obter detalhes do NIF", "detail": err.Error()})
			return
		}
		if len(result.Entries) == 0 {
			context.JSON(http.StatusNotFound, gin.H{"error": "NIF " + request.NIF + " nao encontrado no InfoBio"})
			return
		}
		context.JSON(http.StatusOK, result.Entries[0])
	}
}

func validatedCookies(context *gin.Context, manager *SessionManager) (map[string]string, bool) {
	if manager == nil {
		context.JSON(http.StatusServiceUnavailable, gin.H{"error": "infobio auth is not configured"})
		return nil, false
	}
	userID, ok := userIDFromContext(context)
	if !ok {
		context.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
		return nil, false
	}
	cookies, err := manager.ValidatedCookies(context.Request.Context(), userID)
	if err == nil {
		return cookies, true
	}
	authErr, isAuthErr := err.(*AuthRequiredError)
	if !isAuthErr {
		context.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return nil, false
	}
	context.JSON(http.StatusUnauthorized, gin.H{
		"code":    "infobio_auth_required",
		"expired": authErr.Expired,
		"detail":  authErr.Error(),
	})
	return nil, false
}

func findOneByField(context *gin.Context, service *SearchService, params SearchParams) (Entry, bool, error) {
	result, err := service.Search(context.Request.Context(), params)
	if err != nil {
		return nil, false, err
	}
	if len(result.Entries) == 0 {
		return nil, false, nil
	}
	entry := result.Entries[0]
	if len(result.Entries) > 1 {
		entry = map[string]any{}
		for key, value := range result.Entries[0] {
			entry[key] = value
		}
		entry["_is_list_response"] = true
		entry["_all_entries"] = result.Entries
	}
	return entry, true, nil
}

func userIDFromContext(context *gin.Context) (int64, bool) {
	rawUserID, ok := context.Get("userID")
	if !ok {
		return 0, false
	}
	switch value := rawUserID.(type) {
	case int64:
		return value, true
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
