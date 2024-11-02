package main

import (
	"fmt"
	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
	"html/template"
	"net/http"
	"strconv"
	"subscription-service/data"
)

var pathToManual = "./pdf/manual.pdf"
var tmpPath = "./tmp"

func (app *Config) HomePage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "home.page.gohtml", nil)
}

func (app *Config) LoginPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "login.page.gohtml", nil)
}

func (app *Config) PostLoginPage(w http.ResponseWriter, r *http.Request) {
	_ = app.Session.RenewToken(r.Context())

	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
	}

	email := r.Form.Get("email")
	password := r.Form.Get("password")

	user, err := app.Models.User.GetByEmail(email)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Inavlid login credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	validPassword, err := app.Models.User.PasswordMatches(password)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Inavlid login credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if !validPassword {
		msg := Message{
			To:      email,
			Subject: "Failed login attempt",
			Data:    "Someone tried to login to your account with an incorrect password",
		}

		app.sendMail(&msg)

		app.Session.Put(r.Context(), "error", "Inavlid login credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	app.Session.Put(r.Context(), "userID", user.ID)
	app.Session.Put(r.Context(), "user", user)

	app.Session.Put(r.Context(), "flash", "You have been logged in")

	http.Redirect(w, r, "/", http.StatusSeeOther)

}

func (app *Config) Logout(w http.ResponseWriter, r *http.Request) {
	_ = app.Session.Destroy(r.Context())
	_ = app.Session.RenewToken(r.Context())
	app.Session.Put(r.Context(), "flash", "You have been logged out")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (app *Config) RegisterPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "register.page.gohtml", nil)
}

func (app *Config) PostRegisterPage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
	}

	u := data.User{
		Email:     r.Form.Get("email"),
		Password:  r.Form.Get("password"),
		FirstName: r.Form.Get("first-name"),
		LastName:  r.Form.Get("last-name"),
		Active:    0,
		IsAdmin:   0,
	}

	_, err = app.Models.User.Insert(u)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Failed to create user")
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	url := fmt.Sprintf("http://localhost:80/activate?email=%s", u.Email)
	signedURL := GenerateTokenFromString(url)
	app.InfoLog.Println(signedURL)

	msg := Message{
		To:       u.Email,
		Subject:  "Activate your account",
		Template: "confirmation-email",
		Data:     template.HTML(signedURL),
	}

	app.sendMail(&msg)

	app.Session.Put(r.Context(), "flash", "Please check your email to activate your account")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (app *Config) ActivateAccount(w http.ResponseWriter, r *http.Request) {
	url := r.RequestURI
	testURL := fmt.Sprintf("http://localhost:80%s", url)
	ok := VerifyToken(testURL)
	if !ok {
		app.Session.Put(r.Context(), "error", "Invalid token")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	u, err := app.Models.User.GetByEmail(r.URL.Query().Get("email"))
	if err != nil {
		app.Session.Put(r.Context(), "error", "No user found")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	u.Active = 1
	err = app.Models.User.Update(*u)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Failed to activate account")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	app.Session.Put(r.Context(), "flash", "Account activated")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (app *Config) ChooseSubscription(w http.ResponseWriter, r *http.Request) {
	plans, err := app.Models.Plan.GetAll()
	if err != nil {
		app.ErrorLog.Println(err)
		return
	}

	dataMap := make(map[string]any)
	dataMap["plans"] = plans

	app.render(w, r, "plans.page.gohtml", &TemplateData{
		Data: dataMap,
	})
}

func (app *Config) SubscribeToPlan(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	planID, err := strconv.Atoi(id)
	if err != nil {
		app.ErrorLog.Println(err)
	}

	plan, err := app.Models.Plan.GetOne(planID)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Failed to get plan")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	user, ok := app.Session.Get(r.Context(), "user").(data.User)
	if !ok {
		app.Session.Put(r.Context(), "error", "Login first")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	app.Wait.Add(1)
	go func() {
		defer app.Wait.Done()

		invoice, err := app.getInvoice(user, plan)
		if err != nil {
			app.ErrorChan <- err
			return
		}

		msg := Message{
			To:       user.Email,
			Subject:  "Invoice",
			Data:     invoice,
			Template: "invoice",
		}

		app.sendMail(&msg)
	}()

	app.Wait.Add(1)
	go func() {
		defer app.Wait.Done()

		pdf := app.generateManual(user, plan)
		err := pdf.OutputFileAndClose(fmt.Sprintf("%s/%d_manual.pdf", tmpPath, user.ID))
		if err != nil {
			app.ErrorChan <- err
			return
		}

		msg := Message{
			To:      user.Email,
			Subject: "User Manual",
			Data:    "Please find the user manual attached",
			AttachmentMap: map[string]string{
				"manual.pdf": fmt.Sprintf("%s/%d_manual.pdf", tmpPath, user.ID),
			},
		}

		app.sendMail(&msg)
	}()

	err = app.Models.Plan.SubscribeUserToPlan(user, *plan)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Failed to subscribe to plan")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	u, err := app.Models.User.GetOne(user.ID)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Failed to get user")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	app.Session.Put(r.Context(), "user", u)

	app.Session.Put(r.Context(), "flash", "Please check your email for the invoice and user manual")
	http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
}

func (app *Config) generateManual(u data.User, plan *data.Plan) *gofpdf.Fpdf {
	pdf := gofpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(10, 13, 10)

	importer := gofpdi.NewImporter()

	t := importer.ImportPage(pdf, fmt.Sprintf("%s/manual.pdf", pathToManual), 1, "/MediaBox")
	pdf.AddPage()
	importer.UseImportedTemplate(pdf, t, 0, 0, 215.9, 0)

	pdf.SetX(75)
	pdf.SetY(150)

	pdf.SetFont("Arial", "", 12)
	pdf.MultiCell(0, 4, fmt.Sprintf("%s %s", u.FirstName, u.LastName), "", "C", false)
	pdf.Ln(5)
	pdf.MultiCell(0, 4, fmt.Sprintf("%s User Guide", plan.PlanName), "", "C", false)

	return pdf
}

func (app *Config) getInvoice(u data.User, plan *data.Plan) (string, error) {
	return plan.PlanAmountFormatted, nil
}
