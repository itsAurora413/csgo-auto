package steamauth

import (
    "bytes"
    "crypto/rand"
    "crypto/rsa"
    "crypto/sha1"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "math/big"
    "net/http"
    "net/http/cookiejar"
    "net/url"
    "regexp"
    "time"
)

type Credentials struct {
    Username       string
    Password       string
    SharedSecret   string
    IdentitySecret string
}

type Client struct {
    http *http.Client
}

func NewClient() (*Client, error) {
    jar, _ := cookiejar.New(nil)
    return &Client{http: &http.Client{Timeout: 30 * time.Second, Jar: jar}}, nil
}

// generateSteamGuardCode 生成Steam两步验证码（借鉴Steamauto算法）
func generateSteamGuardCode(sharedSecret string, t time.Time) (string, error) {
    secret, err := base64.StdEncoding.DecodeString(sharedSecret)
    if err != nil {
        return "", err
    }
    timeStep := uint64(t.Unix() / 30)
    var b [8]byte
    for i := uint(0); i < 8; i++ {
        b[7-i] = byte(timeStep & 0xFF)
        timeStep >>= 8
    }
    h := hmacSha1(secret, b[:])
    offset := h[len(h)-1] & 0x0F
    code := (uint32(h[offset])&0x7F)<<24 | (uint32(h[offset+1])&0xFF)<<16 | (uint32(h[offset+2])&0xFF)<<8 | (uint32(h[offset+3]) & 0xFF)
    chars := []rune("23456789BCDFGHJKMNPQRTVWXY")
    var out []rune
    for i := 0; i < 5; i++ {
        out = append(out, chars[code%uint32(len(chars))])
        code /= uint32(len(chars))
    }
    return string(out), nil
}

func hmacSha1(key, data []byte) []byte {
    // Minimal HMAC-SHA1 implementation using crypto/sha1
    blocksize := 64
    if len(key) > blocksize {
        h := sha1.Sum(key)
        key = h[:]
    }
    if len(key) < blocksize {
        key = append(key, bytes.Repeat([]byte{0}, blocksize-len(key))...)
    }
    okey := make([]byte, blocksize)
    ikey := make([]byte, blocksize)
    for i := 0; i < blocksize; i++ {
        okey[i] = key[i] ^ 0x5c
        ikey[i] = key[i] ^ 0x36
    }
    inner := sha1.New()
    inner.Write(ikey)
    inner.Write(data)
    innerSum := inner.Sum(nil)
    outer := sha1.New()
    outer.Write(okey)
    outer.Write(innerSum)
    return outer.Sum(nil)
}

// LoginAndGetAPIKey 按Steamauto流程登录并获取/注册API Key
func (c *Client) LoginAndGetAPIKey(creds Credentials) (string, error) {
    // 1) 获取RSA公钥
    rsakey, ts, err := c.getRSAKey(creds.Username)
    if err != nil {
        return "", err
    }

    // 2) 加密密码
    encPwd, err := encryptPassword(creds.Password, rsakey)
    if err != nil {
        return "", err
    }

    // 3) 生成两步码
    twoFactor, err := generateSteamGuardCode(creds.SharedSecret, time.Now())
    if err != nil {
        return "", err
    }

    // 4) dologin
    if err := c.doLogin(creds.Username, encPwd, twoFactor, ts); err != nil {
        return "", err
    }

    // 5) 获取或注册API Key
    key, err := c.ensureWebAPIKey()
    if err != nil {
        return "", err
    }
    return key, nil
}

func (c *Client) getRSAKey(username string) (*rsa.PublicKey, string, error) {
    form := url.Values{"username": {username}}
    resp, err := c.http.PostForm("https://store.steampowered.com/login/getrsakey/", form)
    if err != nil {
        return nil, "", err
    }
    defer resp.Body.Close()
    var res struct {
        Success      bool   `json:"success"`
        PublicKeyMod string `json:"publickey_mod"`
        PublicKeyExp string `json:"publickey_exp"`
        Timestamp    string `json:"timestamp"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
        return nil, "", err
    }
    if !res.Success {
        return nil, "", errors.New("getrsakey failed")
    }
    n, _ := new(big.Int).SetString(res.PublicKeyMod, 16)
    e, _ := new(big.Int).SetString(res.PublicKeyExp, 16)
    pub := &rsa.PublicKey{N: n, E: int(e.Int64())}
    return pub, res.Timestamp, nil
}

func encryptPassword(password string, pub *rsa.PublicKey) (string, error) {
    enc, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(password))
    if err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(enc), nil
}

func (c *Client) doLogin(username, encPwd, twoFactor, ts string) error {
    form := url.Values{
        "username":        {username},
        "password":        {encPwd},
        "twofactorcode":   {twoFactor},
        "rsatimestamp":    {ts},
        "remember_login":  {"true"},
        "donotcache":      {fmt.Sprintf("%d", time.Now().UnixNano())},
        "oauth_client_id": {"DE45CD61"},
        "oauth_scope":     {"read_profile write_profile read_client write_client"},
    }
    resp, err := c.http.PostForm("https://store.steampowered.com/login/dologin/", form)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    var res struct {
        Success           bool              `json:"success"`
        RequiresTwofactor bool              `json:"requires_twofactor"`
        TransferURLs      []string          `json:"transfer_urls"`
        TransferParams    map[string]string `json:"transfer_parameters"`
        Message           string            `json:"message"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
        return err
    }
    if !res.Success {
        return fmt.Errorf("login failed: %s", res.Message)
    }
    // Complete transfers to set cookies on domains
    for _, u := range res.TransferURLs {
        v := url.Values{}
        for k, val := range res.TransferParams { v.Set(k, val) }
        r, err := c.http.PostForm(u, v)
        if err == nil { io.Copy(io.Discard, r.Body); r.Body.Close() }
    }
    return nil
}

func (c *Client) ensureWebAPIKey() (string, error) {
    // Check existing key
    if key, _ := c.getWebAPIKey(); key != "" { return key, nil }
    // Register new key
    // Need sessionid from cookies
    var sessionID string
    for _, ck := range c.http.Jar.Cookies(&url.URL{Scheme: "https", Host: "steamcommunity.com"}) {
        if ck.Name == "sessionid" { sessionID = ck.Value; break }
    }
    if sessionID == "" {
        // Fallback: from store.steampowered.com
        for _, ck := range c.http.Jar.Cookies(&url.URL{Scheme: "https", Host: "store.steampowered.com"}) {
            if ck.Name == "sessionid" { sessionID = ck.Value; break }
        }
    }
    if sessionID == "" { return "", errors.New("missing sessionid cookie") }
    form := url.Values{
        "sessionid":     {sessionID},
        "agreeToTerms":  {"agreed"},
        "domain":        {"localhost"},
        "Submit":        {"Register"},
    }
    resp, err := c.http.PostForm("https://steamcommunity.com/dev/registerkey", form)
    if err != nil { return "", err }
    defer resp.Body.Close()
    io.Copy(io.Discard, resp.Body)
    // Re-check
    return c.getWebAPIKey()
}

func (c *Client) getWebAPIKey() (string, error) {
    resp, err := c.http.Get("https://steamcommunity.com/dev/apikey")
    if err != nil { return "", err }
    defer resp.Body.Close()
    b, _ := io.ReadAll(resp.Body)
    re := regexp.MustCompile(`Key:\s*([0-9A-F]{32})`)
    m := re.FindSubmatch(b)
    if len(m) >= 2 { return string(m[1]), nil }
    return "", nil
}

// AcceptTradeOffer 使用WebAPI接受报价
func (c *Client) AcceptTradeOffer(apiKey, offerID string) error {
    form := url.Values{
        "key":          {apiKey},
        "tradeofferid": {offerID},
    }
    resp, err := c.http.PostForm("https://api.steampowered.com/IEconService/AcceptTradeOffer/v1/", form)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("accept failed: %s - %s", resp.Status, string(b))
    }
    return nil
}
