package moviepoll

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zorchenhimer/MoviePolls/common"
)

// Returns current active votes and votes for watched movies
func (s *Server) getUserVotes(user *common.User) ([]*common.Movie, []*common.Movie, error) {
	voted, err := s.data.GetUserVotes(user.Id)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to get all user votes for ID %d: %v", user.Id, err)
	}

	current := []*common.Movie{}
	watched := []*common.Movie{}

	for _, movie := range voted {
		if movie.Removed == true {
			continue
		}

		if movie.CycleWatched == nil {
			current = append(current, movie)
		} else {
			watched = append(watched, movie)
		}
	}

	return current, watched, nil
}

func (s *Server) handlerUser(w http.ResponseWriter, r *http.Request) {
	user := s.getSessionUser(w, r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	totalVotes, err := s.data.GetCfgInt("MaxUserVotes", DefaultMaxUserVotes)
	if err != nil {
		s.l.Error("Error getting MaxUserVotes config setting: %v", err)
		totalVotes = DefaultMaxUserVotes
	}

	activeVotes, watchedVotes, err := s.getUserVotes(user)
	if err != nil {
		s.l.Error("Unable to get votes for user %d: %v", user.Id, err)
	}

	addedMovies, err := s.data.GetUserMovies(user.Id)
	if err != nil {
		s.l.Error("Unable to get movies added by user %d: %v", user.Id, err)
	}

	unlimited, err := s.data.GetCfgBool(ConfigUnlimitedVotes, DefaultUnlimitedVotes)
	if err != nil {
		s.l.Error("Error getting %s config setting: %v", ConfigUnlimitedVotes, err)
	}

	data := struct {
		dataPageBase

		TotalVotes     int
		AvailableVotes int
		UnlimitedVotes bool

		ActiveVotes    []*common.Movie
		WatchedVotes   []*common.Movie
		AddedMovies    []*common.Movie
		SuccessMessage string

		PassError   []string
		NotifyError []string
		EmailError  []string

		ErrCurrentPass bool
		ErrNewPass     bool
		ErrEmail       bool
	}{
		dataPageBase: s.newPageBase("Account", w, r),

		TotalVotes:     totalVotes,
		AvailableVotes: totalVotes - len(activeVotes),
		UnlimitedVotes: unlimited,

		ActiveVotes:  activeVotes,
		WatchedVotes: watchedVotes,
		AddedMovies:  addedMovies,
	}

	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			s.l.Error("ParseForm() error: %v", err)
			s.doError(http.StatusInternalServerError, "Form error", w, r)
			return
		}

		formVal := r.PostFormValue("Form")
		if formVal == "ChangePassword" {
			// Do password stuff
			currentPass := s.hashPassword(r.PostFormValue("PasswordCurrent"))
			newPass1_raw := r.PostFormValue("PasswordNew1")
			newPass2_raw := r.PostFormValue("PasswordNew2")

			if currentPass != user.Password {
				data.ErrCurrentPass = true
				data.PassError = append(data.PassError, "Invalid current password")
			}

			if newPass1_raw == "" {
				data.ErrNewPass = true
				data.PassError = append(data.PassError, "New password cannot be blank")
			}

			if newPass1_raw != newPass2_raw {
				data.ErrNewPass = true
				data.PassError = append(data.PassError, "Passwords do not match")
			}

			if !(data.ErrCurrentPass || data.ErrNewPass || data.ErrEmail) {
				// Change pass
				data.SuccessMessage = "Password successfully changed"
				user.Password = s.hashPassword(newPass1_raw)
				user.PassDate = time.Now()

				s.l.Info("new PassDate: %s", user.PassDate)

				err = s.login(user, w, r)
				if err != nil {
					s.l.Error("Unable to login to session:", err)
					s.doError(http.StatusInternalServerError, "Unable to update password", w, r)
					return
				}

				if err = s.data.UpdateUser(user); err != nil {
					s.l.Error("Unable to save User with new password:", err)
					s.doError(http.StatusInternalServerError, "Unable to update password", w, r)
					return
				}
			}

		} else if formVal == "Notifications" {
			// Update notifications
		}
	}

	if err := s.executeTemplate(w, "account", data); err != nil {
		s.l.Error("Error rendering template: %v", err)
	}
}
func (s *Server) handlerUserLogin(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		s.l.Error("Error parsing login form: %v", err)
	}

	user := s.getSessionUser(w, r)
	if user != nil {
		http.Redirect(w, r, "/user", http.StatusFound)
		return
	}

	data := dataLoginForm{}
	doRedirect := false

	if r.Method == "POST" {
		// do login

		un := r.PostFormValue("Username")
		pw := r.PostFormValue("Password")
		user, err = s.data.UserLogin(un, s.hashPassword(pw))
		if err != nil {
			data.ErrorMessage = err.Error()
		} else {
			doRedirect = true
		}

	} else {
		s.l.Info("> no post: %s", r.Method)
	}

	if user != nil {
		err = s.login(user, w, r)
		if err != nil {
			s.l.Error("Unable to login: %v", err)
			s.doError(http.StatusInternalServerError, "Unable to login", w, r)
			return
		}
	}

	// Redirect to base page on successful login
	if doRedirect {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	data.dataPageBase = s.newPageBase("Login", w, r) // set this last to get correct login status

	if err := s.executeTemplate(w, "simplelogin", data); err != nil {
		s.l.Error("Error rendering template: %v", err)
	}
}

func (s *Server) handlerUserLogout(w http.ResponseWriter, r *http.Request) {
	err := s.logout(w, r)
	if err != nil {
		s.l.Error("Error logging out: %v", err)
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handlerUserNew(w http.ResponseWriter, r *http.Request) {
	user := s.getSessionUser(w, r)
	if user != nil {
		http.Redirect(w, r, "/account", http.StatusFound)
		return
	}

	data := struct {
		dataPageBase

		ErrorMessage []string
		ErrName      bool
		ErrPass      bool
		ErrEmail     bool

		ValName           string
		ValEmail          string
		ValNotifyEnd      bool
		ValNotifySelected bool
	}{
		dataPageBase: s.newPageBase("Create Account", w, r),
	}

	doRedirect := false

	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			s.l.Error("Error parsing login form: %v", err)
			data.ErrorMessage = append(data.ErrorMessage, err.Error())
		}

		un := strings.TrimSpace(r.PostFormValue("Username"))
		data.ValName = un

		// TODO: password requirements
		pw1 := r.PostFormValue("Password1")
		pw2 := r.PostFormValue("Password2")

		data.ValName = un

		if un == "" {
			data.ErrorMessage = append(data.ErrorMessage, "Username cannot be blank!")
			data.ErrName = true
		}

		maxlen, err := s.data.GetCfgInt(ConfigMaxNameLength, DefaultMaxNameLength)
		if err != nil {
			s.doError(http.StatusInternalServerError, "Something went wrong :C", w, r)
			s.l.Error("Unable to get MaxNameLength config value: %v", err)
			return
		}

		minlen, err := s.data.GetCfgInt(ConfigMinNameLength, DefaultMinNameLength)
		if err != nil {
			s.doError(http.StatusInternalServerError, "Something went wrong :C", w, r)
			s.l.Error("Unable to get MinNameLength config value: %v", err)
			return
		}

		s.l.Debug("New user: %s (%d) maxlen: %d", un, len(un), maxlen)

		if len(un) > maxlen {
			data.ErrorMessage = append(data.ErrorMessage, fmt.Sprintf("Username cannot be longer than %d characters", maxlen))
			data.ErrName = true
		}

		if len(un) < minlen {
			data.ErrorMessage = append(data.ErrorMessage, fmt.Sprintf("Username cannot be shorter than %d characters", minlen))
			data.ErrName = true
		}

		if pw1 != pw2 {
			data.ErrorMessage = append(data.ErrorMessage, "Passwords do not match!")
			data.ErrPass = true

		} else if pw1 == "" {
			data.ErrorMessage = append(data.ErrorMessage, "Password cannot be blank!")
			data.ErrPass = true
		}

		notifyEnd := r.PostFormValue("NotifyEnd")
		notifySelected := r.PostFormValue("NotifySelected")
		email := r.PostFormValue("Email")

		data.ValEmail = email
		if notifyEnd != "" {
			data.ValNotifyEnd = true
		}

		if notifySelected != "" {
			data.ValNotifySelected = true
		}

		if (notifyEnd != "" || notifySelected != "") && email == "" {
			data.ErrEmail = true
			data.ErrorMessage = append(data.ErrorMessage, "Email required for notifications")
		}

		if len(data.ErrorMessage) == 0 {
			newUser := &common.User{
				Name:                un,
				Password:            s.hashPassword(pw1),
				Email:               email,
				NotifyCycleEnd:      data.ValNotifyEnd,
				NotifyVoteSelection: data.ValNotifySelected,
				PassDate:            time.Now(),
			}

			_, err = s.data.AddUser(newUser)
			if err != nil {
				data.ErrorMessage = append(data.ErrorMessage, err.Error())
			} else {
				err = s.login(newUser, w, r)
				if err != nil {
					s.l.Error("Unable to login to session: %v", err)
					s.doError(http.StatusInternalServerError, "Login error", w, r)
					return
				}
				doRedirect = true
			}
		}
	}

	if doRedirect {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if err := s.executeTemplate(w, "newaccount", data); err != nil {
		s.l.Error("Error rendering template: %v", err)
	}
}
