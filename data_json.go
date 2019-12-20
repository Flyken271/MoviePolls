package moviepoll

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//type jsonCycle
type jsonMovie struct {
	Id           int
	Name         string
	Links        []string
	Description  string
	CycleAddedId int
	Removed      bool
	Approved     bool
	Watched      *time.Time
	Poster       string
}

func (j *jsonConnector) newJsonMovie(movie *Movie) jsonMovie {
	currentCycle := j.currentCycle()
	cycleId := 0
	if currentCycle != nil {
		cycleId = currentCycle.Id
	}

	return jsonMovie{
		Id:           j.nextMovieId(),
		Name:         movie.Name,
		Links:        movie.Links,
		Description:  movie.Description,
		CycleAddedId: cycleId,
		Removed:      movie.Removed,
		Approved:     movie.Approved,
		Poster:       movie.Poster,
	}
}

type jsonVote struct {
	UserId  int
	MovieId int
	CycleId int
}

type jsonConnector struct {
	filename string `json:"-"`
	lock     *sync.RWMutex

	CurrentCycle int

	Cycles []*Cycle
	Movies []jsonMovie
	Users  []*User
	Votes  []jsonVote

	//Settings Configurator
	Settings map[string]configValue
}

func NewJsonConnector(filename string) (*jsonConnector, error) {
	if fileExists(filename) {
		return LoadJson(filename)
	}

	return &jsonConnector{
		filename:     filename,
		lock:         &sync.RWMutex{},
		CurrentCycle: 0,
		Settings: map[string]configValue{
			"Active": configValue{CVT_BOOL, true},
		},
	}, nil
}

func LoadJson(filename string) (*jsonConnector, error) {
	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	data := &jsonConnector{}
	err = json.Unmarshal(raw, data)
	if err != nil {
		return nil, fmt.Errorf("Unable to read JSON data: %v", err)
	}

	data.filename = filename
	data.lock = &sync.RWMutex{}

	return data, nil
}

func (j *jsonConnector) save() error {
	raw, err := json.MarshalIndent(j, "", " ")
	if err != nil {
		return fmt.Errorf("Unable to marshal JSON data: %v", err)
	}

	err = ioutil.WriteFile(j.filename, raw, 0777)
	if err != nil {
		return fmt.Errorf("Unable to write JSON data: %v", err)
	}

	return nil
}

/*
   On determining the current cycle.

   Should the current cycle have an end date?
   If so, this would be the automatic end date for the cycle.
   If not, only the current cycle would have an end date, which would define
   the current cycle as the cycle without an end date.

   Otherwise, just store the current cycle's ID somewhere (current
   functionality).
*/
func (j *jsonConnector) currentCycle() *Cycle {

	for _, c := range j.Cycles {
		if j.CurrentCycle == c.Id {
			return c
		}
	}
	return nil
}

func (j *jsonConnector) GetCurrentCycle() *Cycle {
	j.lock.RLock()
	defer j.lock.RUnlock()

	return j.currentCycle()
}

func (j *jsonConnector) AddCycle(end *time.Time) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	if j.Cycles == nil {
		j.Cycles = []*Cycle{}
	}

	c := &Cycle{
		Id:    j.nextCycleId(),
		Start: time.Now(),
	}

	if end != nil {
		c.End = *end
		c.EndingSet = true
	} else {
		c.EndingSet = false
	}
	j.Cycles = append(j.Cycles, c)

	return c.Id, j.save()
}

func (j *jsonConnector) AddOldCycle(c *Cycle) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	if j.Cycles == nil {
		j.Cycles = []*Cycle{}
	}

	c.Id = j.nextCycleId()

	j.Cycles = append(j.Cycles, c)
	return c.Id, j.save()
}

func (j *jsonConnector) nextCycleId() int {
	highest := 0
	for _, c := range j.Cycles {
		if c.Id > highest {
			highest = c.Id
		}
	}
	return highest + 1
}

func (j *jsonConnector) nextMovieId() int {
	highest := 0
	for _, m := range j.Movies {
		if m.Id > highest {
			highest = m.Id
		}
	}
	return highest + 1
}

func (j *jsonConnector) AddMovie(movie *Movie) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	fmt.Printf("Adding movie %s\n", movie.String())
	if j.Movies == nil {
		j.Movies = []jsonMovie{}
	}

	m := j.newJsonMovie(movie)
	j.Movies = append(j.Movies, m)

	return m.Id, j.save()
}

func (j *jsonConnector) GetMovie(id int) (*Movie, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	movie := j.findMovie(id)
	if movie == nil {
		return nil, fmt.Errorf("Movie with ID %d not found.", id)
	}

	movie.Votes = j.findVotes(movie)
	return movie, nil
}

func (j *jsonConnector) GetActiveMovies() []*Movie {
	j.lock.RLock()
	defer j.lock.RUnlock()

	movies := []*Movie{}

	for _, m := range j.Movies {
		mov, _ := j.GetMovie(m.Id)
		if mov != nil && m.Watched == nil {
			movies = append(movies, mov)
		}
	}

	return movies
}

func (j *jsonConnector) GetPastCycles(start, end int) []*Cycle {
	// TODO: implement this
	return []*Cycle{}
}

// UserLogin returns a user if the given username and password match a user.
func (j *jsonConnector) UserLogin(name, hashedPw string) (*User, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	name = strings.ToLower(name)
	for _, user := range j.Users {
		if strings.ToLower(user.Name) == name {
			if hashedPw == user.Password {
				return user, nil
			}
			fmt.Printf("Bad password for user %s\n", name)
			return nil, fmt.Errorf("Invalid login credentials")
		}
	}
	fmt.Printf("User with name %s not found\n", name)
	return nil, fmt.Errorf("Invalid login credentials")
}

// Get the total number of users
func (j *jsonConnector) GetUserCount() int {
	j.lock.RLock()
	defer j.lock.RUnlock()

	return len(j.Users)
}

func (j *jsonConnector) GetUsers(start, count int) ([]*User, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	uids := []int{}
	for _, u := range j.Users {
		uids = append(uids, u.Id)
	}

	sort.Ints(uids)

	ulist := []*User{}
	for i := 0; i < len(uids) && len(ulist) <= count; i++ {
		id := uids[i]
		if id < start {
			continue
		}

		u := j.findUser(id)
		if u != nil {
			ulist = append(ulist, u)
		}
	}

	return ulist, nil
}

func (j *jsonConnector) GetUser(userId int) (*User, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	u := j.findUser(userId)
	if u == nil {
		return nil, fmt.Errorf("User not found with ID %s", userId)
	}
	return u, nil
}

func (j *jsonConnector) GetUserVotes(userId int) []*Movie {
	j.lock.RLock()
	defer j.lock.RUnlock()

	votes := []*Movie{}
	for _, v := range j.Votes {
		if v.UserId == userId {
			mov := j.findMovie(v.MovieId)
			if mov != nil {
				votes = append(votes, mov)
			}
		}
	}
	return votes
}

func (j *jsonConnector) nextUserId() int {
	highest := 0
	for _, u := range j.Users {
		if u.Id > highest {
			highest = u.Id
		}
	}
	return highest + 1
}

func (j *jsonConnector) AddUser(user *User) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	name := strings.ToLower(user.Name)
	for _, u := range j.Users {
		if u.Id == user.Id {
			return 0, fmt.Errorf("User already exists with ID %d", user.Id)
		}
		if strings.ToLower(u.Name) == name {
			return 0, fmt.Errorf("User already exists with name %s", user.Name)
		}
	}

	user.Id = j.nextUserId()

	j.Users = append(j.Users, user)
	return user.Id, j.save()
}

func (j *jsonConnector) AddVote(userId, movieId int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	user := j.findUser(userId)
	if user == nil {
		return fmt.Errorf("User not found with ID %d", userId)
	}

	movie := j.findMovie(movieId)
	if movie == nil {
		return fmt.Errorf("Movie not found with ID %d", movieId)
	}

	if movie.Watched != nil {
		return fmt.Errorf("Movie has already been watched")
	}

	if movie.Removed {
		return fmt.Errorf("Movie has been removed by a mod or admin")
	}

	// TODO: check for movie approval

	cc := j.currentCycle()
	cId := 0
	if cc != nil {
		cId = cc.Id
	}

	j.Votes = append(j.Votes, jsonVote{userId, movieId, cId})
	return j.save()
}

func (j *jsonConnector) findMovie(id int) *Movie {
	for _, m := range j.Movies {
		if m.Id == id {
			return &Movie{
				Id:          id,
				Name:        m.Name,
				Description: m.Description,
				Removed:     m.Removed,
				Approved:    m.Approved,
				CycleAdded:  j.findCycle(m.CycleAddedId),
				Links:       m.Links,
				Poster:      m.Poster,
			}
		}
	}

	return nil
}

func (j *jsonConnector) CheckMovieExists(title string) bool {
	j.lock.RLock()
	defer j.lock.RUnlock()

	clean := cleanMovieName(title)
	for _, m := range j.Movies {
		if clean == cleanMovieName(m.Name) {
			return true
		}
	}
	return false
}

func (j *jsonConnector) CheckUserExists(name string) bool {
	j.lock.RLock()
	defer j.lock.RUnlock()

	lc := strings.ToLower(name)
	for _, user := range j.Users {
		if lc == strings.ToLower(user.Name) {
			return true
		}
	}
	return false
}

func (j *jsonConnector) findCycle(id int) *Cycle {
	for _, c := range j.Cycles {
		if c.Id == id {
			return c
		}
	}
	return nil
}

func (j *jsonConnector) findVotes(movie *Movie) []*Vote {
	votes := []*Vote{}
	for _, v := range j.Votes {
		if v.MovieId == movie.Id {
			votes = append(votes, &Vote{
				Movie:      movie,
				CycleAdded: j.findCycle(v.CycleId),
				User:       j.findUser(v.UserId),
			})
		}
	}

	return votes
}

func (j *jsonConnector) findUser(id int) *User {
	for _, u := range j.Users {
		if u.Id == id {
			return u
		}
	}
	return nil
}

func (j *jsonConnector) UpdateUser(user *User) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	newLst := []*User{}
	for _, u := range j.Users {
		if u.Id == user.Id {
			newLst = append(newLst, user)
		} else {
			newLst = append(newLst, u)
		}
	}
	return j.save()
}

func (j *jsonConnector) UpdateMovie(movie *Movie) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	newLst := []jsonMovie{}
	for _, m := range j.Movies {
		if m.Id == movie.Id {
			newLst = append(newLst, j.newJsonMovie(movie))
		} else {
			newLst = append(newLst, m)
		}
	}
	return j.save()
}

func (j *jsonConnector) UpdateCycle(cycle *Cycle) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	newLst := []*Cycle{}
	for _, c := range j.Cycles {
		if c.Id == cycle.Id {
			newLst = append(newLst, cycle)
		} else {
			newLst = append(newLst, c)
		}
	}
	return j.save()
}

func (j *jsonConnector) UserVotedForMovie(userId, movieId int) bool {
	j.lock.RLock()
	defer j.lock.RUnlock()

	for _, v := range j.Votes {
		if v.MovieId == movieId && v.UserId == userId {
			return true
		}
	}

	return false
}

// Configuration stuff
type cfgValType int

const (
	CVT_STRING cfgValType = iota
	CVT_INT
	CVT_BOOL
)

type configValue struct {
	Type  cfgValType
	Value interface{}
}

func (v configValue) String() string {
	t := ""
	switch v.Type {
	case CVT_STRING:
		t = "string"
		break
	case CVT_INT:
		t = "int"
		break
	case CVT_BOOL:
		t = "bool"
		break
	}

	return fmt.Sprintf("configValue{Type:%s Value:%v}", t, v.Value)
}

func (j *jsonConnector) GetCfgString(key string) (string, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	val, ok := j.Settings[key]
	if !ok {
		return "", fmt.Errorf("Setting with key %q does not exist", key)
	}

	switch val.Type {
	case CVT_STRING:
		return val.Value.(string), nil
	case CVT_INT:
		return fmt.Sprintf("%d", val.Value.(int)), nil
	case CVT_BOOL:
		return fmt.Sprintf("%t", val.Value.(bool)), nil
	default:
		return "", fmt.Errorf("Unknown type %d", val.Type)
	}
}

func (j *jsonConnector) GetCfgInt(key string) (int, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	val, ok := j.Settings[key]
	if !ok {
		return 0, fmt.Errorf("Setting with key %q does not exist", key)
	}

	switch val.Type {
	case CVT_STRING:
		ival, err := strconv.ParseInt(val.Value.(string), 10, 32)
		if err != nil {
			return 0, fmt.Errorf("Int parse error: %s", err)
		}

		return int(ival), nil
	case CVT_INT:
		if val, ok := val.Value.(int); ok {
			return val, nil
		}
		if val, ok := val.Value.(float64); ok {
			return int(val), nil
		}
		return 0, fmt.Errorf("Unknown number type for %s", key)
	case CVT_BOOL:
		if val.Value.(bool) == true {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("Unknown type %d", val.Type)
	}
}

func (j *jsonConnector) GetCfgBool(key string) (bool, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	val, ok := j.Settings[key]
	if !ok {
		return false, fmt.Errorf("Setting with key %q does not exist", key)
	}

	switch val.Type {
	case CVT_STRING:
		bval, err := strconv.ParseBool(val.Value.(string))
		if err != nil {
			return false, fmt.Errorf("Bool parse error: %s", err)
		}
		return bval, nil
	case CVT_INT:
		if val.Value.(int) == 0 {
			return false, nil
		}
		return true, nil
	case CVT_BOOL:
		return val.Value.(bool), nil
	default:
		return false, fmt.Errorf("Unknown type %d", val.Type)
	}
}

func (j *jsonConnector) SetCfgString(key, value string) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.Settings[key] = configValue{CVT_STRING, value}

	return j.save()
}

func (j *jsonConnector) SetCfgInt(key string, value int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.Settings[key] = configValue{CVT_INT, value}

	return j.save()
}

func (j *jsonConnector) SetCfgBool(key string, value bool) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.Settings[key] = configValue{CVT_BOOL, value}

	return j.save()
}

func (j *jsonConnector) DeleteCfgKey(key string) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	delete(j.Settings, key)

	return j.save()
}