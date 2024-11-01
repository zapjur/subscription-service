package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfig_AddDefaultData(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := getCtx(req)
	req = req.WithContext(ctx)

	testApp.Session.Put(ctx, "flash", "flash")
	testApp.Session.Put(ctx, "warning", "warning")
	testApp.Session.Put(ctx, "error", "error")

	td := testApp.AddDefaultData(&TemplateData{}, req)

	if td.Flash != "flash" {
		t.Error("flash value not found in session")
	}
	if td.Warning != "warning" {
		t.Error("warning value not found in session")
	}
	if td.Error != "error" {
		t.Error("error value not found in session")
	}
}

func TestConfig_IsAuthenticated(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := getCtx(req)
	req = req.WithContext(ctx)

	isAuth := testApp.isAuthenticated(req)
	if isAuth {
		t.Error("request is authenticated, should not be")
	}

	testApp.Session.Put(ctx, "userID", 1)
	isAuth = testApp.isAuthenticated(req)
	if !isAuth {
		t.Error("request is not authenticated, should be")
	}
}

func TestConfig_render(t *testing.T) {
	pathToTemplates = "./templates"

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := getCtx(req)
	req = req.WithContext(ctx)

	testApp.render(rr, req, "home.page.gohtml", &TemplateData{})

	if rr.Code != 200 {
		t.Errorf("Expected response code 200, got %d", rr.Code)
	}
}
