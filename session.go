package moviepoll

import (
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/zorchenhimer/MoviePolls/common"
)

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	session, err := s.cookies.Get(r, SessionName)
	if err != nil {
		return fmt.Errorf("Unable to get session from store: %v", err)
	}

	return delSession(session, w, r)
}

func (s *Server) login(user *common.User, w http.ResponseWriter, r *http.Request) error {
	session, err := s.cookies.Get(r, SessionName)
	if err != nil {
		return fmt.Errorf("Unable to get session from store: %v", err)
	}

	gobbed, err := user.PassDate.GobEncode()
	if err != nil {
		return fmt.Errorf("Unable to gob PassDate")
	}

	session.Values["UserId"] = user.Id
	session.Values["PassDate"] = fmt.Sprintf("%X", sha256.Sum256([]byte(gobbed)))

	return session.Save(r, w)
}

func delSession(session *sessions.Session, w http.ResponseWriter, r *http.Request) error {
	delete(session.Values, "UserId")
	delete(session.Values, "PassDate")

	return session.Save(r, w)
}

func (s *Server) getSessionUser(w http.ResponseWriter, r *http.Request) *common.User {
	session, err := s.cookies.Get(r, SessionName)
	if err != nil {
		s.l.Error("Unable to get session from store: %v", err)
		err = delSession(session, w, r)
		if err != nil {
			s.l.Error("Unable to delete cookie: %v", err)
		}
		return nil
	}

	val := session.Values["UserId"]
	var userId int
	var ok bool

	if userId, ok = val.(int); !ok {
		err = delSession(session, w, r)
		if err != nil {
			s.l.Error("Unable to delete cookie: %v", err)
		}
		return nil
	}

	user, err := s.data.GetUser(userId)
	if err != nil {
		s.l.Error("Unable to get user with ID %d: %v", userId, err)
		err = delSession(session, w, r)
		if err != nil {
			s.l.Error("Unable to delete cookie: %v", err)
		}
		return nil
	}

	passDate, _ := session.Values["PassDate"].(string)
	gobbed, err := user.PassDate.GobEncode()

	if err != nil || fmt.Sprintf("%X", sha256.Sum256([]byte(gobbed))) != passDate {
		s.l.Info("User's PassDate did not match stored value")
		err = delSession(session, w, r)
		if err != nil {
			s.l.Error("Unable to delete cookie: %v", err)
		}
		return nil
	}

	return user
}
