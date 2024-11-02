package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"subscription-service/data"
	"testing"
)

var pageTests = []struct {
	name                string
	url                 string
	expcectedStatusCode int
	handler             http.HandlerFunc
	sessionData         map[string]any
	expectedHTML        string
}{
	{
		name:                "home",
		url:                 "/",
		expcectedStatusCode: http.StatusOK,
		handler:             testApp.HomePage,
	},
	{
		name:                "login",
		url:                 "/login",
		expcectedStatusCode: http.StatusOK,
		handler:             testApp.LoginPage,
		expectedHTML:        `<h1 class="mt-5">Login</h1>`,
	},
	{
		name:                "logout",
		url:                 "/logout",
		expcectedStatusCode: http.StatusSeeOther,
		handler:             testApp.Logout,
		sessionData: map[string]any{
			"userID": 1,
			"user":   data.User{},
		},
	},
}

func Test_Pages(t *testing.T) {
	pathToTemplates = "./templates"

	for _, e := range pageTests {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", e.url, nil)

		ctx := getCtx(req)
		req = req.WithContext(ctx)

		if len(e.sessionData) > 0 {
			for key, value := range e.sessionData {
				testApp.Session.Put(ctx, key, value)
			}
		}

		e.handler.ServeHTTP(rr, req)

		if rr.Code != e.expcectedStatusCode {
			t.Errorf("Home page returned %v, expected %v", rr.Code, e.expcectedStatusCode)
		}

		if len(e.expectedHTML) > 0 {
			html := rr.Body.String()
			if !strings.Contains(html, e.expectedHTML) {
				t.Errorf("Expected HTML: %s, got: %s", e.expectedHTML, html)
			}
		}
	}
}

func TestConfig_PostLoginPage(t *testing.T) {
	pathToTemplates = "./templates"

	postedData := url.Values{
		"email":    {"admin@example.com"},
		"password": {"admin"},
	}

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", strings.NewReader(postedData.Encode()))
	ctx := getCtx(req)
	req = req.WithContext(ctx)

	handler := http.HandlerFunc(testApp.PostLoginPage)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("PostLoginPage returned %v, expected %v", rr.Code, http.StatusSeeOther)
	}

	if !testApp.Session.Exists(ctx, "userID") {
		t.Error("Did not get userID in session")
	}
}

func TestConfig_SubscribeToPlan(t *testing.T) {
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/subscribe?id=1", nil)
	ctx := getCtx(req)
	req = req.WithContext(ctx)

	testApp.Session.Put(ctx, "user", data.User{
		ID:        1,
		Email:     "admin@example.com",
		FirstName: "admin",
		LastName:  "admin",
		Active:    1,
	})

	handler := http.HandlerFunc(testApp.SubscribeToPlan)
	handler.ServeHTTP(rr, req)

	testApp.Wait.Wait()

	if rr.Code != http.StatusSeeOther {
		t.Errorf("SubscribeToPlan returned %v, expected %v", rr.Code, http.StatusSeeOther)
	}
}
