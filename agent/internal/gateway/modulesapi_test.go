package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeLocalModules struct {
	forceErr bool
}

func (f fakeLocalModules) ListModules() []ModuleStatusDTO {
	return []ModuleStatusDTO{
		{ID: "power-monitor", Label: "Power Monitor", Version: "1.0.0", Origin: "local"},
	}
}

func (f fakeLocalModules) StageModule(_ context.Context, _, _ []byte) (StagePreviewDTO, error) {
	if f.forceErr {
		return StagePreviewDTO{}, errors.New("stage failed")
	}
	return StagePreviewDTO{
		StageID: "stage-abc", ID: "power-monitor", Version: "1.0.0", Label: "Power Monitor",
	}, nil
}

func (f fakeLocalModules) ConfirmModule(_ context.Context, _, _ string) error {
	if f.forceErr {
		return errors.New("confirm failed")
	}
	return nil
}

func (f fakeLocalModules) UninstallModule(_ context.Context, _ string) error {
	if f.forceErr {
		return errors.New("uninstall failed")
	}
	return nil
}

func newModulesGW(lm LocalModules) *Gateway {
	return New("nats://x", "rover", "tok", "", nil, nil, "", lm)
}

// buildMultipart assembles a multipart/form-data body with two file fields.
func buildMultipart(t *testing.T, rawData, bundleData []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range []struct{ field string; data []byte }{{field: "raw", data: rawData}, {field: "bundle", data: bundleData}} {
		fw, err := mw.CreateFormFile(f.field, f.field+".bin")
		require.NoError(t, err)
		_, err = fw.Write(f.data)
		require.NoError(t, err)
	}
	require.NoError(t, mw.Close())
	return &buf, mw.FormDataContentType()
}

func TestModulesAPI_List_Unauthorized(t *testing.T) {
	gw := newModulesGW(fakeLocalModules{})
	srv := httptest.NewServer(gw.HTTPHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/modules")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestModulesAPI_List_OK(t *testing.T) {
	gw := newModulesGW(fakeLocalModules{})
	srv := httptest.NewServer(gw.HTTPHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/modules?token=tok")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var list []ModuleStatusDTO
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	require.Len(t, list, 1)
	require.Equal(t, "power-monitor", list[0].ID)
}

func TestModulesAPI_Stage_OK(t *testing.T) {
	gw := newModulesGW(fakeLocalModules{})
	srv := httptest.NewServer(gw.HTTPHandler())
	defer srv.Close()

	body, ct := buildMultipart(t, []byte("rawbytes"), []byte("bundlebytes"))
	req, err := http.NewRequest("POST", srv.URL+"/api/modules/stage?token=tok", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var preview StagePreviewDTO
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&preview))
	require.Equal(t, "power-monitor", preview.ID)
}

func TestModulesAPI_Stage_Error(t *testing.T) {
	gw := newModulesGW(fakeLocalModules{forceErr: true})
	srv := httptest.NewServer(gw.HTTPHandler())
	defer srv.Close()

	body, ct := buildMultipart(t, []byte("rawbytes"), []byte("bundlebytes"))
	req, err := http.NewRequest("POST", srv.URL+"/api/modules/stage?token=tok", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestModulesAPI_Confirm_OK(t *testing.T) {
	gw := newModulesGW(fakeLocalModules{})
	srv := httptest.NewServer(gw.HTTPHandler())
	defer srv.Close()

	body := strings.NewReader(`{"stageId":"s","configToml":""}`)
	req, err := http.NewRequest("POST", srv.URL+"/api/modules/confirm?token=tok", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestModulesAPI_Confirm_Error(t *testing.T) {
	gw := newModulesGW(fakeLocalModules{forceErr: true})
	srv := httptest.NewServer(gw.HTTPHandler())
	defer srv.Close()

	body := strings.NewReader(`{"stageId":"s","configToml":""}`)
	req, err := http.NewRequest("POST", srv.URL+"/api/modules/confirm?token=tok", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestModulesAPI_Uninstall_OK(t *testing.T) {
	gw := newModulesGW(fakeLocalModules{})
	srv := httptest.NewServer(gw.HTTPHandler())
	defer srv.Close()

	req, err := http.NewRequest("DELETE", srv.URL+"/api/modules/umr?token=tok", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestModulesAPI_NilLocalModules_Returns503(t *testing.T) {
	gw := newModulesGW(nil)
	srv := httptest.NewServer(gw.HTTPHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/modules?token=tok")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
