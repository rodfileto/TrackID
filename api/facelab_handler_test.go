package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid-vision/vision"
)

// fakeVision finds one face whose landmarks sit where the test image draws its eyes.
type fakeVision struct{ eyes [2][2]float32 }

func (f fakeVision) DetectFaces(io.Reader) ([]vision.Detection, error) {
	e := f.eyes
	return []vision.Detection{{Box: [4]float32{e[0][0] - 30, e[0][1] - 40, e[1][0] + 30, e[0][1] + 90}, Score: 0.9,
		Kps: [5][2]float32{e[0], e[1], {(e[0][0] + e[1][0]) / 2, e[0][1] + 30}, {e[0][0] + 5, e[0][1] + 55}, {e[1][0] - 5, e[1][1] + 55}}}}, nil
}

func (fakeVision) DetectAndEmbed(io.Reader) ([]vision.Face, error) { return nil, nil }

func faceLabEngine(lab *FaceLab) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api")
	g.GET("facelab", lab.FaceInfoHandler())
	g.POST("facelab/sources", lab.FaceUploadHandler())
	g.POST("facelab/render", lab.FaceRenderHandler())
	g.POST("facelab/align", lab.FaceAlignHandler())
	g.POST("facelab/check", lab.FaceCheckHandler())
	g.POST("facelab/sheet", lab.FaceSheetHandler())
	return r
}

func facePNG(t *testing.T, w, h int, shade uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{shade, uint8(x % 200), uint8(y % 200), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func call(t *testing.T, r http.Handler, method, path string, body any) map[string]any {
	t.Helper()
	var req *http.Request
	if b, ok := body.([]byte); ok {
		var form bytes.Buffer
		mw := multipart.NewWriter(&form)
		part, _ := mw.CreateFormFile("file", "face.png")
		part.Write(b)
		mw.Close()
		req = httptest.NewRequest(method, path, &form)
		req.Header.Set("Content-Type", mw.FormDataContentType())
	} else {
		j, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(j))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s: %d %v", method, path, rec.Code, out)
	}
	return out
}

func TestFaceLabEndToEnd(t *testing.T) {
	lab := NewFaceLab(fakeVision{eyes: [2][2]float32{{100, 120}, {160, 120}}})
	r := faceLabEngine(lab)

	info := call(t, r, http.MethodGet, "/api/facelab", nil)
	if len(info["components"].([]any)) != 19 || len(info["scale"].([]any)) != 5 || info["detection"] != true {
		t.Fatalf("info: %v components, %v levels", len(info["components"].([]any)), len(info["scale"].([]any)))
	}
	q := call(t, r, http.MethodPost, "/api/facelab/sources", facePNG(t, 260, 300, 90))
	k := call(t, r, http.MethodPost, "/api/facelab/sources", facePNG(t, 260, 300, 160))
	faces := q["faces"].([]any)
	if len(faces) != 1 {
		t.Fatalf("faces %v", faces)
	}
	landmarks := map[string]any{"points": faces[0].(map[string]any)["landmarks"], "source": "DETECTOR"}

	render := call(t, r, http.MethodPost, "/api/facelab/render", map[string]any{"sourceId": k["id"],
		"recipe": map[string]any{"rotate": 5, "scale": 0.5, "grey": "luminance"}})
	if !strings.Contains(render["record"].(string), "Redimensionamento uniforme pelo fator 0,500") {
		t.Errorf("record: %s", render["record"])
	}

	side := func(src map[string]any, recipe map[string]any) map[string]any {
		return map[string]any{"sourceId": src["id"], "recipe": recipe, "landmarks": landmarks,
			"analysis": map[string]any{"done": true, "suitability": "SUITABLE"}}
	}
	kRecipe := map[string]any{"scale": 0.5}
	align := call(t, r, http.MethodPost, "/api/facelab/align", map[string]any{"questioned": side(q, nil), "known": side(k, kRecipe)})
	a := align["alignment"].(map[string]any)
	if s := a["scale"].(float64); s < 1.99 || s > 2.01 {
		t.Errorf("a known image rendered at half size should align at scale 2, got %v", s)
	}

	session := map[string]any{
		"questioned": side(q, nil), "known": side(k, kRecipe), "questionedFirst": true,
		"pairs": align["pairs"], "superimposition": true, "factors": []string{"LONG_TERM"},
		"markups": []map[string]any{
			{"image": "Q", "number": 1, "kind": "POINT", "points": [][2]float64{{130, 150}}},
			{"image": "K", "number": 1, "kind": "POINT", "points": [][2]float64{{130, 150}}},
		},
		"findings": []map[string]any{{"characteristic": "NASAL_TIP", "result": "SIMILAR", "questioned": "Ponta idêntica", "markups": []int{1}}},
		"opinion":  map[string]any{"conclusion": "SUPPORT_COMMON", "reasoning": "Semelhanças estáveis."},
	}
	check := call(t, r, http.MethodPost, "/api/facelab/check", map[string]any{"session": session})
	codes := ""
	for _, w := range check["warnings"].([]any) {
		codes += w.(map[string]any)["code"].(string) + " "
	}
	if codes != "WORDING_FEATURE " {
		t.Errorf("warnings: %s", codes)
	}
	sheet := call(t, r, http.MethodPost, "/api/facelab/sheet", map[string]any{"session": session})
	for _, want := range []string{"face.png", "Ponta nasal: semelhante", "escala 2,000", "Suporte à origem comum", sheet["annotatedSha256"].(string)} {
		if !strings.Contains(sheet["record"].(string), want) {
			t.Errorf("the record lacks %q", want)
		}
	}
	if !strings.HasPrefix(sheet["clean"].(string), "data:image/png;base64,") {
		t.Error("no clean sheet")
	}
}
