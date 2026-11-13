package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"

	"github.com/mythologyli/zju-connect/log"
)

type PasswordLogin struct {
	PasswordProvider func(context.Context) (string, error)
	Username         string
	Password         string
	Domain           string
	GraphCodeFile    string
}

func (m PasswordLogin) AuthType() string {
	return "auth/psw"
}

func (m PasswordLogin) LoginDomain() string {
	return m.Domain
}

func (m PasswordLogin) login(s *Session, _ AuthInfo) error {
	if m.PasswordProvider != nil {
		password, err := m.PasswordProvider(s.ctx)
		if err != nil {
			return err
		}
		m.Password = password
	}
	return s.loginAuthPsw(m.Username, m.Password, m.Domain, m.GraphCodeFile)
}

func (s *Session) loginAuthPsw(username, password, loginDomain, graphCodeFile string) error {
	process := func(graphCheckCode string) (int, error) {
		return s.pswImpl(username, password, loginDomain, graphCheckCode)
	}
	return s.withGraphCheckCode(process, graphCodeFile)
}

func (s *Session) pswImpl(username, password, loginDomain, graphCheckCode string) (int, error) {
	log.Println("Perform POST /passport/v1/auth/psw")

	pub, err := s.rsaPublicKey()
	if err != nil {
		return 0, err
	}

	msg := []byte(password + "_" + s.antiReplayRand)
	cipherBytes, err := encryptPKCS1v15Chunks(pub, msg)
	if err != nil {
		return 0, err
	}
	encryptedPwd := hex.EncodeToString(cipherBytes)

	s.username = username + "@" + loginDomain
	data := map[string]interface{}{
		"username":    s.username,
		"password":    encryptedPwd,
		"rememberPwd": "0",
	}

	if graphCheckCode != "" {
		data["graphCheckCode"] = graphCheckCode
	}
	postBody, _ := json.Marshal(data)

	u := s.baseURL + "/passport/v1/auth/psw"
	req, _ := http.NewRequest("POST", u+"?"+WithSharedParams(nil).Encode(), bytes.NewReader(postBody))
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	req.Header.Set("x-csrf-token", s.csrfToken)
	req.Header.Set("x-sdp-env", s.env)
	req.Header.Set("x-sdp-traceid", s.randSdpId())

	resp, err := s.request(req)
	if err != nil {
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, _ := io.ReadAll(resp.Body)

	var re struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Ticket               string `json:"ticket"`
			GraphCheckCodeEnable int    `json:"graphCheckCodeEnable"`
			NextService          string `json:"nextService"`
			AntiReplayRand       string `json:"antiReplayRand"`
		} `json:"data"`
	}
	err = json.Unmarshal(body, &re)
	if err != nil {
		return 0, err
	}
	if re.Code != 0 {
		if re.Data.GraphCheckCodeEnable == 1 {
			return re.Data.GraphCheckCodeEnable, nil
		}
		return 0, gatewayAuthError("password authentication failed", re.Code)
	}

	if re.Data.Ticket == "" && re.Data.GraphCheckCodeEnable == 0 {
		return 0, fmt.Errorf("password authentication succeeded without a ticket")
	}
	s.ticket = re.Data.Ticket

	s.nextService = re.Data.NextService
	// The server issues a fresh antiReplayRand; the challenge token must use it (stale -> 75500000).
	if re.Data.AntiReplayRand != "" {
		s.antiReplayRand = re.Data.AntiReplayRand
	}

	return re.Data.GraphCheckCodeEnable, nil
}

func encryptPKCS1v15Chunks(pub *rsa.PublicKey, plaintext []byte) ([]byte, error) {
	if pub == nil || pub.N == nil || pub.N.BitLen() < 512 || pub.N.BitLen() > 8192 || pub.E < 3 || pub.E > 1<<31-1 || len(plaintext) > 32768 {
		return nil, fmt.Errorf("RSA input exceeds supported bounds")
	}
	maxChunkSize := pub.Size() - 11
	if maxChunkSize <= 0 {
		return nil, fmt.Errorf("RSA public key is too small")
	}

	ciphertext := make([]byte, 0, ((len(plaintext)+maxChunkSize-1)/maxChunkSize)*pub.Size())
	for len(plaintext) > 0 {
		chunkSize := min(len(plaintext), maxChunkSize)
		chunk, err := rsa.EncryptPKCS1v15(rand.Reader, pub, plaintext[:chunkSize])
		if err != nil {
			return nil, err
		}
		ciphertext = append(ciphertext, chunk...)
		plaintext = plaintext[chunkSize:]
	}
	return ciphertext, nil
}

// encryptChallengeResponse encrypts a code as "<code>_<antiReplayRand>" via PKCS#1 v1.5, like the password.
func (s *Session) encryptChallengeResponse(code string) (string, error) {
	pub, err := s.rsaPublicKey()
	if err != nil {
		return "", err
	}
	cipherBytes, err := encryptPKCS1v15Chunks(pub, []byte(code+"_"+s.antiReplayRand))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(cipherBytes), nil
}

func (s *Session) rsaPublicKey() (*rsa.PublicKey, error) {
	if len(s.pubKey) > 2048 {
		return nil, fmt.Errorf("RSA modulus exceeds size limit")
	}
	N, ok := new(big.Int).SetString(s.pubKey, 16)
	if !ok || N.BitLen() < 512 || N.BitLen() > 8192 {
		return nil, fmt.Errorf("invalid RSA public key modulus")
	}
	E, err := strconv.Atoi(s.pubKeyExp)
	if err != nil || E < 3 || E > 1<<31-1 {
		return nil, fmt.Errorf("invalid RSA public key exponent")
	}
	return &rsa.PublicKey{N: N, E: E}, nil
}
