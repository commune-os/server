package app

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"github.com/sony/sonyflake"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

type AccessToken struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

func ExtractAccessToken(req *http.Request) (*AccessToken, error) {
	authBearer := req.Header.Get("Authorization")

	if authBearer != "" {
		parts := strings.SplitN(authBearer, " ", 2)
		if len(parts) != 2 ||
			(parts[0] != "Bearer" && parts[0] != "Bot") {
			return nil, errors.New("Invalid Authorization header.")
		}
		return &AccessToken{
			Type:  parts[0],
			Token: parts[1],
		}, nil
	}

	return nil, errors.New("Missing access token.")
}

type WellKnownServer struct {
	ServerName string `json:"server_name,omitempty"`
}

func WellKnown(s string) (*WellKnownServer, error) {
	resp, err := http.Get(fmt.Sprintf(`%s/.well-known/matrix/server`, s))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)

	var res WellKnownServer

	err = json.Unmarshal(bodyBytes, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

type UserID struct {
	LocalPart  string
	ServerName string
}

// Determind whether username belongs to this homesever or an external one
// external usernames should be in full @username:homeserver.org form
// local usernames should be @username only for better UI
func (c *App) FederationUser(username string) (bool, *UserID) {
	// validUsernameRegex := regexp.MustCompile(`^@[0-9a-z_\-=./]+:[0-9a-z_\-=./]+\.[a-z]{2,}$`)
	validUsernameRegex := regexp.MustCompile(`^@.+?:.+$`)
	if validUsernameRegex.MatchString(username) {
		username = username[1:]
		parts := strings.Split(username, ":")

		if parts[1] == c.Config.Matrix.Homeserver {
			return false, nil
		}
		return true, &UserID{
			LocalPart:  parts[0],
			ServerName: parts[1],
		}
	}

	return false, nil
}

func FederationRoom(username string) (bool, *UserID) {
	// validUsernameRegex := regexp.MustCompile(`^@[0-9a-z_\-=./]+:[0-9a-z_\-=./]+\.[a-z]{2,}$`)
	validUsernameRegex := regexp.MustCompile(`^.+?:.+$`)
	if validUsernameRegex.MatchString(username) {
		parts := strings.Split(username, ":")
		log.Println(parts)
		return true, &UserID{
			LocalPart:  parts[0],
			ServerName: parts[1],
		}
	}

	return false, nil
}

func GetHomeServerPart(s string) string {
	if strings.Contains(s, ":") {
		sp := strings.Split(s, ":")
		return sp[len(sp)-1]
	}
	return s
}

func GetHomeserverFromAlias(alias string) string {
	parts := strings.Split(alias, ":")
	lastPart := parts[len(parts)-1]
	if strings.Contains(lastPart, ":") {
		return strings.Join(parts[1:len(parts)-1], ":")
	}
	return strings.Join(parts[1:], ":")
}

func (c *App) IsFederated(username string) (bool, *UserID) {
	// federated user paths should have the same format as email, like so: username@homeserver.com
	// obviously a very loose regexp
	// validUsernameRegex := regexp.MustCompile(`^.+?@.+$`)
	validUsernameRegex := regexp.MustCompile(`^@.+?:.+$`)
	if validUsernameRegex.MatchString(username) {

		// lets's split the localpart and server_name
		parts := strings.Split(username, ":")

		// if severname is the same as out homeserver, return
		if parts[1] == c.Config.App.Domain ||
			parts[1] == c.Config.Matrix.Homeserver {
			return false, nil
		}

		// return federated path
		return true, &UserID{
			LocalPart:  parts[0],
			ServerName: parts[1],
		}
	}

	return false, nil
}

func FileID(fileID string) string {
	fi := strings.Replace(fileID, "mxc://", "", 1)
	sp := strings.Split(fi, "/")
	return sp[1]
}

func (c *App) URLScheme(url string) string {
	if c.Config.Matrix.Homeserver != url &&
		c.Config.Matrix.FederationServer != url {
		return fmt.Sprintf(`https://%s`, url)
	}
	return fmt.Sprintf(`http://%s`, url)
}

func UnsafeHTML(x string) (template.HTML, error) {
	unsafe := blackfriday.Run([]byte(x))
	return template.HTML(unsafe), nil
}

func ToHTML(x string) (template.HTML, error) {
	unsafe := blackfriday.Run([]byte(x))
	p := bluemonday.UGCPolicy()
	safe := p.Sanitize(string(unsafe))
	return template.HTML(safe), nil
}

func ToStrictHTML(x string) (template.HTML, error) {
	unsafe := blackfriday.Run([]byte(x))

	p := bluemonday.NewPolicy()
	p.AllowStandardURLs()
	p.RequireParseableURLs(true)
	p.AllowRelativeURLs(true)

	p.AllowStandardAttributes()

	p.AllowImages()

	p.AllowURLSchemes("mailto", "https")

	p.AllowAttrs("href").OnElements("a")

	p.AllowElements("blockquote")

	p.AllowElements("p")
	p.AllowElements("b", "strong")
	p.AllowElements("i", "em")
	p.AllowAttrs("class").OnElements("span")

	p.AllowElements("br")

	p.AllowElements("hr")
	p.AllowElements("ul")
	p.AllowElements("ol")
	p.AllowElements("li")
	p.AllowElements("br")

	p.AllowAttrs("id").OnElements("li")
	p.AllowAttrs("class").OnElements("li")

	p.AllowElements("sub")
	p.AllowElements("sup")

	p.AllowElements("s")
	p.AllowElements("del")

	p.AllowElements("pre")
	p.AllowElements("code")

	safe := p.Sanitize(string(unsafe))
	return template.HTML(safe), nil
}

func SanitizeHTML(x string) (string, error) {
	p := bluemonday.StrictPolicy()
	p.AllowElements("br")
	safe := p.Sanitize(x)
	return safe, nil
}

func StrictSanitizeHTML(x string) (string, error) {
	p := bluemonday.StrictPolicy()
	safe := p.Sanitize(x)
	return safe, nil
}

const (
	letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	NumberBytes = "0123456789"
)
const (
	letterIdxBits = 6
	letterIdxMask = 1<<letterIdxBits - 1
	letterIdxMax  = 63 / letterIdxBits
)

func RandomString(n int) string {
	src := rand.NewSource(time.Now().UnixNano())
	b := make([]byte, n)

	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func RandomNumber(n int) string {
	b := make([]byte, n)

	src := rand.NewSource(time.Now().UnixNano())
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(NumberBytes) {
			b[i] = NumberBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func StripMXCPrefix(s string) string {
	s = strings.Replace(s, "mxc://", "", -1)
	return s
}

func (c *App) RoomPathFromAlias(alias string) string {
	federated := false
	sp := strings.Split(alias, ":")
	if sp[1] != c.Config.App.Domain {
		federated = true
	}

	path := ""
	rp := alias[1:]

	if !federated {
		sp := strings.Split(rp, ":")
		s := strings.Split(sp[0], "_")
		if len(sp) > 1 {
			p := strings.Join(s, "/")
			path = p
		} else {
			path = s[0]
		}
	} else {
		path = rp
	}
	return path
}

func FormatTime(t time.Time) string {
	thisYear := "Jan _2"
	pastYears := "Jan _2, 2006"
	// max := 24 * time.Hour

	now := time.Now()

	difference := now.Sub(t)

	// If it's within last 12 hours

	if difference < time.Minute {
		return "Just Now"
	}

	if difference < time.Hour {
		difference = difference.Round(time.Minute)
		x := math.Trunc(difference.Minutes())
		return fmt.Sprintf(`%.fm`, x)
	}

	if difference <= time.Hour*23 {
		difference = difference.Round(time.Hour)
		x := math.Trunc(difference.Hours())
		return fmt.Sprintf(`%.fh`, x)
	}

	if t.Year() == now.Year() {
		x := t.Format(thisYear)
		return x
	}

	return t.Format(pastYears)
}

func GetLocalPart(s string) string {
	s = s[1:]
	x := strings.Split(s, ":")
	return x[0]
}

func (c *App) GetLocalPartPath(s string, profile bool) string {
	s = s[1:]
	x := strings.Split(s, ":")

	g := strings.Split(x[0], "_")

	if profile {
		g = g[1:]
	}

	if !strings.Contains(x[1], c.Config.App.Domain) && !profile {
		g[0] = fmt.Sprintf(`%s:%s`, g[0], x[1])
	}

	return strings.Join(g, "/")
}

type NewUser struct {
	Username string
	Password string
	Admin    bool
}

func ConstructMac(u *NewUser, nonce, secret string) (string, error) {
	admin := "notadmin"
	if u.Admin {
		admin = "admin"
	}

	joined := strings.Join([]string{nonce, u.Username, u.Password, admin}, "\x00")

	mac := hmac.New(sha1.New, []byte(secret))
	_, err := mac.Write([]byte(joined))
	if err != nil {
		log.Println(err)
		return "", err
	}

	sha := hex.EncodeToString(mac.Sum(nil))

	return sha, nil
}

func GetNonce(domain string) (string, error) {
	resp, err := http.Get(fmt.Sprintf(`%s/_synapse/admin/v1/register`, domain))
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)

	type Response struct {
		Nonce string `json:"nonce"`
	}

	var res Response

	err = json.Unmarshal(bodyBytes, &res)
	if err != nil {
		return "", err
	}

	return res.Nonce, nil
}

var flake = sonyflake.NewSonyflake(sonyflake.Settings{})

func genSonyflake() uint64 {
	id, err := flake.NextID()
	if err != nil {
		log.Println(err)
	}
	// Note: this is base16, could shorten by encoding as base62 string
	return id
}

func SliceContains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

// generate 6 digit magic code
func GenerateMagicCode() string {
	rand.Seed(time.Now().UnixNano())
	return fmt.Sprintf("%06d", rand.Intn(999999))
}

// lifted from Dendrite
func GenerateAccessToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// url-safe no padding
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// split email address into local and domain parts
func SplitEmail(email string) (string, string) {
	parts := strings.Split(email, "@")
	return parts[0], parts[1]
}

func encodeUUID(src [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", src[0:4], src[4:6], src[6:8], src[8:10], src[10:16])
}

func IsValidAlias(input string) bool {
	reg := regexp.MustCompile("^[a-zA-Z0-9-]+$")
	return reg.MatchString(input)
}

func removeElement(arr []string, element string) []string {
	index := -1
	for i, value := range arr {
		if value == element {
			index = i
			break
		}
	}
	if index != -1 {
		result := append([]string{}, arr[:index]...)
		result = append(result, arr[index+1:]...)
		return result
	}
	return arr
}

func ConvertToJSON(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			v[key] = ConvertToJSON(val)
		}
		return v
	case []interface{}:
		for i, val := range v {
			v[i] = ConvertToJSON(val)
		}
		return v
	default:
		return data
	}
}
