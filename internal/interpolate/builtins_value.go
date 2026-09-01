package interpolate

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// resolveArg turns a builtin argument into a value.
//
// An argument is first tried as a reference into the interpolation context —
// "captured.foo", a variable name, an environment key — and falls back to being
// a literal. That is what lets built-ins be written without a second layer of
// braces: {{$jsonPath(captured.result, promotionId)}}.
func resolveArg(ctx *tryve.InterpolationContext, arg string) any {
	if ctx == nil {
		return arg
	}
	if val, err := evalExpression(arg, ctx); err == nil {
		return val
	}
	return arg
}

// builtinJSON parses its argument as JSON and returns the decoded value.
//
// A step that captures a script's stdout holds a JSON document as a string;
// this turns it back into data that paths and assertions can address.
func builtinJSON(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("json: expected 1 argument (the value to parse), got %d", len(args))
	}
	val := resolveArg(ctx, args[0])
	if _, isString := val.(string); !isString {
		// Already structured — nothing to parse.
		return val, nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(val.(string)), &decoded); err != nil {
		return nil, fmt.Errorf("json: value is not valid JSON: %w", err)
	}
	return decoded, nil
}

// builtinJSONPath reads a dotted path out of a value, parsing JSON strings on
// the way down.
//
//	{{$jsonPath(captured.setup_result, promotionId)}}
//	{{$jsonPath(captured.response, data.items[0].id)}}
func builtinJSONPath(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("jsonPath: expected 2 arguments (value, path), got %d", len(args))
	}
	root := resolveArg(ctx, args[0])
	path := strings.TrimPrefix(strings.TrimPrefix(args[1], "$"), ".")

	val, found := Lookup(root, path)
	if !found {
		return nil, fmt.Errorf("jsonPath: path %q not found in value", args[1])
	}
	return val, nil
}

// builtinJSONFile reads a JSON file and optionally returns one path within it.
//
//	{{$jsonFile(local.settings.json, Values.INTERNAL_JWT_KEYS)}}
//
// This replaces shelling out to `cat file | jq -r .some.key`.
func builtinJSONFile(_ *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("jsonFile: expected 1 or 2 arguments (path[, jsonPath]), got %d", len(args))
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		return nil, fmt.Errorf("jsonFile: could not read %q: %w", args[0], err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("jsonFile: %q does not contain valid JSON: %w", args[0], err)
	}
	if len(args) == 1 {
		return decoded, nil
	}
	path := strings.TrimPrefix(strings.TrimPrefix(args[1], "$"), ".")
	val, found := Lookup(decoded, path)
	if !found {
		return nil, fmt.Errorf("jsonFile: path %q not found in %q", args[1], args[0])
	}
	return val, nil
}

// builtinInt coerces a value to an integer.
func builtinInt(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("int: expected 1 argument, got %d", len(args))
	}
	val := resolveArg(ctx, args[0])
	switch n := val.(type) {
	case float64:
		return int64(n), nil
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("int: %q is not an integer", n)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("int: cannot convert %T to an integer", val)
	}
}

// builtinNumber coerces a value to a floating-point number.
func builtinNumber(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("number: expected 1 argument, got %d", len(args))
	}
	val := resolveArg(ctx, args[0])
	switch n := val.(type) {
	case float64:
		return n, nil
	case int64:
		return float64(n), nil
	case int:
		return float64(n), nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return nil, fmt.Errorf("number: %q is not a number", n)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("number: cannot convert %T to a number", val)
	}
}

// builtinBool coerces a value to a boolean.
func builtinBool(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("bool: expected 1 argument, got %d", len(args))
	}
	val := resolveArg(ctx, args[0])
	switch b := val.(type) {
	case bool:
		return b, nil
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(b))
		if err != nil {
			return nil, fmt.Errorf("bool: %q is not a boolean", b)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("bool: cannot convert %T to a boolean", val)
	}
}

// builtinDefault returns its first argument when that argument resolves, and the
// second otherwise. It is the escape hatch for optional values under strict
// resolution: {{$default(captured.token, anonymous)}}.
func builtinDefault(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("default: expected 2 arguments (value, fallback), got %d", len(args))
	}
	if ctx != nil {
		if val, err := evalExpression(args[0], ctx); err == nil && val != nil {
			if s, isStr := val.(string); !isStr || s != "" {
				return val, nil
			}
		}
	}
	return resolveArg(ctx, args[1]), nil
}

// builtinBase64URL encodes a value with unpadded base64url, the encoding JWTs use.
func builtinBase64URL(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("base64url: expected 1 argument, got %d", len(args))
	}
	return base64.RawURLEncoding.EncodeToString([]byte(Stringify(resolveArg(ctx, args[0])))), nil
}

// builtinHMAC returns the hex HMAC-SHA256 of a message under a key.
func builtinHMAC(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("hmac: expected 2 arguments (message, key), got %d", len(args))
	}
	msg := Stringify(resolveArg(ctx, args[0]))
	key := Stringify(resolveArg(ctx, args[1]))
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(msg))
	return fmt.Sprintf("%x", mac.Sum(nil)), nil
}

// builtinJWT signs a JSON Web Token, replacing the shell scripts test suites
// otherwise need to mint an access token for every run.
//
//	{{$jwt(HS256, secret, {"sub":"123"}, 1h)}}
//	{{$jwt(RS256, {{$jsonFile(keys.json, private)}}, {"sub":"123"}, 1h, my-key-id)}}
//
// Arguments: algorithm, key, claims (a JSON object), optional lifetime, optional
// key id. "iat" and "exp" are filled in from the lifetime unless the claims
// already set them.
func builtinJWT(ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if len(args) < 3 || len(args) > 5 {
		return nil, fmt.Errorf(
			"jwt: expected 3 to 5 arguments (algorithm, key, claims[, lifetime][, keyId]), got %d", len(args))
	}

	alg := strings.ToUpper(strings.TrimSpace(args[0]))
	key := Stringify(resolveArg(ctx, args[1]))

	claims, err := jwtClaims(ctx, args[2])
	if err != nil {
		return nil, err
	}

	lifetime := time.Hour
	if len(args) >= 4 && strings.TrimSpace(args[3]) != "" {
		lifetime, err = time.ParseDuration(strings.TrimSpace(args[3]))
		if err != nil {
			return nil, fmt.Errorf("jwt: invalid lifetime %q: %w", args[3], err)
		}
	}
	now := time.Now()
	if _, set := claims["iat"]; !set {
		claims["iat"] = now.Unix()
	}
	if _, set := claims["exp"]; !set {
		claims["exp"] = now.Add(lifetime).Unix()
	}

	header := map[string]any{"alg": alg, "typ": "JWT"}
	if len(args) == 5 && strings.TrimSpace(args[4]) != "" {
		header["kid"] = strings.TrimSpace(args[4])
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("jwt: could not encode header: %w", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("jwt: could not encode claims: %w", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)

	signature, err := signJWT(alg, key, signingInput)
	if err != nil {
		return nil, err
	}
	return signingInput + "." + signature, nil
}

// jwtClaims resolves the claims argument into a map.
func jwtClaims(ctx *tryve.InterpolationContext, arg string) (map[string]any, error) {
	val := resolveArg(ctx, arg)
	if m, ok := val.(map[string]any); ok {
		// Copy so the caller's captured data is not mutated by iat/exp defaults.
		claims := make(map[string]any, len(m)+2)
		for k, v := range m {
			claims[k] = v
		}
		return claims, nil
	}
	var claims map[string]any
	if err := json.Unmarshal([]byte(Stringify(val)), &claims); err != nil {
		return nil, fmt.Errorf("jwt: claims must be a JSON object: %w", err)
	}
	return claims, nil
}

// signJWT produces the signature segment for the given algorithm.
func signJWT(alg, key, signingInput string) (string, error) {
	switch alg {
	case "HS256":
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(signingInput))
		return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil

	case "RS256":
		priv, err := parseRSAPrivateKey(key)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256([]byte(signingInput))
		sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest[:])
		if err != nil {
			return "", fmt.Errorf("jwt: RS256 signing failed: %w", err)
		}
		return base64.RawURLEncoding.EncodeToString(sig), nil

	default:
		return "", fmt.Errorf("jwt: unsupported algorithm %q; supported: HS256, RS256", alg)
	}
}

// parseRSAPrivateKey accepts a PEM private key, or the base64 of one, which is
// how key material is commonly carried in environment variables.
func parseRSAPrivateKey(key string) (*rsa.PrivateKey, error) {
	material := strings.TrimSpace(key)
	if !strings.Contains(material, "-----BEGIN") {
		decoded, err := base64.StdEncoding.DecodeString(material)
		if err != nil {
			return nil, fmt.Errorf("jwt: key is neither PEM nor base64-encoded PEM: %w", err)
		}
		material = string(decoded)
	}

	block, _ := pem.Decode([]byte(material))
	if block == nil {
		return nil, fmt.Errorf("jwt: key does not contain a PEM block")
	}

	if priv, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return priv, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("jwt: could not parse private key: %w", err)
	}
	priv, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("jwt: RS256 requires an RSA private key, got %T", parsed)
	}
	return priv, nil
}
