package controller

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildZip returns an in-memory zip containing the given name->content entries.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// zipUploadRequest builds a multipart POST carrying the zip under field "file".
func zipUploadRequest(t *testing.T, target string, zipBytes []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "batch.zip")
	require.NoError(t, err)
	_, err = fw.Write(zipBytes)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestImportPoolAuthFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Fake pool: accepts multipart, reports every file part as uploaded.
	var receivedParts int
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(32<<20))
		for _, hs := range r.MultipartForm.File {
			receivedParts += len(hs)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","uploaded":` + strconv.Itoa(receivedParts) + `,"files":[],"failed":[]}`))
	}))
	defer pool.Close()

	t.Setenv("POOL_K12_MGMT_URL", pool.URL)
	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")

	zipBytes := buildZip(t, map[string]string{
		"alive/a@x.com-k12-1.json": `{"type":"codex","email":"a@x.com","access_token":"t","account_id":"id"}`,
		"alive/note.txt":           `not json`,
		"alive/bad.json":           `{not valid json`,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = zipUploadRequest(t, "/api/pool/auth-files/import?pool=k12", zipBytes)

	ImportPoolAuthFiles(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Imported int          `json:"imported"`
			Skipped  []importSkip `json:"skipped"`
			Failed   []importFail `json:"failed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, 1, resp.Data.Imported, "only the one valid json forwards")
	assert.Equal(t, 1, receivedParts, "pool receives exactly one file part")
	require.Len(t, resp.Data.Skipped, 2, "the .txt and the malformed json are skipped")
	for _, s := range resp.Data.Skipped {
		assert.NotEmpty(t, s.Reason, "each skip carries a reason")
	}
}

func TestImportPoolAuthFilesUnconfiguredPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pool/auth-files/import?pool=k12", nil)
	ImportPoolAuthFiles(c)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestParsePoolAuthAccounts(t *testing.T) {
	t.Run("bare codex object derives name from email", func(t *testing.T) {
		items := parsePoolAuthAccounts(`{"email":"a@b.com","access_token":"tok"}`, "")
		require.Len(t, items, 1)
		assert.Equal(t, "codex-a-b-com.json", items[0].name)
		assert.Contains(t, items[0].content, "a@b.com")
	})
	t.Run("bare object keeps caller-supplied name", func(t *testing.T) {
		items := parsePoolAuthAccounts(`{"email":"a@b.com"}`, "codex-custom.json")
		require.Len(t, items, 1)
		assert.Equal(t, "codex-custom.json", items[0].name)
	})
	t.Run("exporter bundle expands every account", func(t *testing.T) {
		blob := `{"type":"x","version":1,"accounts":[
			{"name":"one","credentials":{"email":"one@x.com","access_token":"t1"}},
			{"name":"two","credentials":{"email":"two@x.com","access_token":"t2"}},
			{"credentials":{"email":"three@x.com","access_token":"t3"}}
		]}`
		items := parsePoolAuthAccounts(blob, "ignored.json")
		require.Len(t, items, 3)
		assert.ElementsMatch(t,
			[]string{"codex-one-x-com.json", "codex-two-x-com.json", "codex-three-x-com.json"},
			[]string{items[0].name, items[1].name, items[2].name})
		// Each item carries only the inner credential, not the account wrapper.
		assert.Contains(t, items[0].content, "access_token")
	})
	t.Run("json array of bare objects expands", func(t *testing.T) {
		blob := `[{"email":"x@y.com","access_token":"t"},{"email":"z@y.com","access_token":"t2"}]`
		items := parsePoolAuthAccounts(blob, "")
		require.Len(t, items, 2)
		assert.ElementsMatch(t, []string{"codex-x-y-com.json", "codex-z-y-com.json"},
			[]string{items[0].name, items[1].name})
	})
	t.Run("account without email falls back to account name then index", func(t *testing.T) {
		blob := `{"accounts":[{"name":"Alpha","credentials":{"access_token":"t"}},{"credentials":{"access_token":"t2"}}]}`
		items := parsePoolAuthAccounts(blob, "")
		require.Len(t, items, 2)
		assert.Equal(t, "codex-alpha.json", items[0].name)
		assert.Equal(t, "codex-account-2.json", items[1].name)
	})
	t.Run("non-JSON passes through unchanged for the pool to reject", func(t *testing.T) {
		items := parsePoolAuthAccounts("not json", "raw.json")
		require.Len(t, items, 1)
		assert.Equal(t, "raw.json", items[0].name)
		assert.Equal(t, "not json", items[0].content)
	})
}
