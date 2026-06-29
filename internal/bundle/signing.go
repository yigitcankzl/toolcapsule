package bundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"toolcapsule/internal/manifest"
)

const (
	DigestFileName    = "capsule.digest.json"
	SignatureFileName = "signature.json"
)

type KeygenOptions struct {
	Out    string
	PubOut string
}

type KeygenResult struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key,omitempty"`
	KeyID      string `json:"key_id"`
}

type VerifyOptions struct {
	PublicKey string
}

type VerifyResult struct {
	OK         bool     `json:"ok"`
	Path       string   `json:"path"`
	Signed     bool     `json:"signed"`
	Verified   bool     `json:"verified"`
	Tool       string   `json:"tool,omitempty"`
	SourceHash string   `json:"source_hash,omitempty"`
	PublicKey  string   `json:"public_key,omitempty"`
	KeyID      string   `json:"key_id,omitempty"`
	Files      int      `json:"files"`
	Warnings   []string `json:"warnings,omitempty"`
}

type keyFile struct {
	Algorithm  string `json:"algorithm"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key,omitempty"`
}

type digestFile struct {
	Version         int               `json:"version"`
	DigestAlgorithm string            `json:"digest_algorithm"`
	CreatedAt       string            `json:"created_at"`
	Files           []fileDigestEntry `json:"files"`
}

type fileDigestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type signatureFile struct {
	Version   int    `json:"version"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

func GenerateKeyPair(opts KeygenOptions) (KeygenResult, error) {
	if opts.Out == "" {
		return KeygenResult{}, fmt.Errorf("keygen requires --out")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeygenResult{}, err
	}
	publicHex := hex.EncodeToString(publicKey)
	privateHex := hex.EncodeToString(privateKey)
	privateData, err := json.MarshalIndent(keyFile{Algorithm: "ed25519", PublicKey: publicHex, PrivateKey: privateHex}, "", "  ")
	if err != nil {
		return KeygenResult{}, err
	}
	if err := writeFile(opts.Out, privateData, 0o600); err != nil {
		return KeygenResult{}, err
	}
	if opts.PubOut != "" {
		publicData, err := json.MarshalIndent(keyFile{Algorithm: "ed25519", PublicKey: publicHex}, "", "  ")
		if err != nil {
			return KeygenResult{}, err
		}
		if err := writeFile(opts.PubOut, publicData, 0o644); err != nil {
			return KeygenResult{}, err
		}
	}
	return KeygenResult{PrivateKey: opts.Out, PublicKey: opts.PubOut, KeyID: keyID(publicHex)}, nil
}

func SignEntries(entries map[string][]byte, keyPath string) ([]byte, []byte, string, error) {
	privateKey, publicHex, err := loadPrivateKey(keyPath)
	if err != nil {
		return nil, nil, "", err
	}
	digest := digestForEntries(entries)
	digest.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	digestData, err := json.MarshalIndent(digest, "", "  ")
	if err != nil {
		return nil, nil, "", err
	}
	signature := ed25519.Sign(privateKey, digestData)
	signatureData, err := json.MarshalIndent(signatureFile{
		Version:   1,
		Algorithm: "ed25519",
		PublicKey: publicHex,
		KeyID:     keyID(publicHex),
		Signature: hex.EncodeToString(signature),
	}, "", "  ")
	if err != nil {
		return nil, nil, "", err
	}
	return digestData, signatureData, publicHex, nil
}

func VerifyBundle(path string, opts VerifyOptions) (VerifyResult, error) {
	extracted, cleanup, err := Extract(path)
	if err != nil {
		return VerifyResult{}, err
	}
	defer cleanup()
	result, err := VerifyDir(extracted.Dir, opts)
	result.Path = path
	return result, err
}

func VerifyDir(dir string, opts VerifyOptions) (VerifyResult, error) {
	result := VerifyResult{OK: true, Path: dir}
	if m, err := manifest.Load(filepath.Join(dir, manifest.FileName)); err == nil {
		result.Tool = m.Name
		result.SourceHash = m.Capsule.SourceHash
	}

	digestData, digestErr := os.ReadFile(filepath.Join(dir, DigestFileName))
	signatureData, signatureErr := os.ReadFile(filepath.Join(dir, SignatureFileName))
	if os.IsNotExist(digestErr) && os.IsNotExist(signatureErr) {
		if opts.PublicKey != "" {
			return result, fmt.Errorf("bundle is unsigned")
		}
		result.Signed = false
		result.Verified = false
		result.Warnings = append(result.Warnings, "bundle is unsigned")
		return result, nil
	}
	if digestErr != nil {
		return result, digestErr
	}
	if signatureErr != nil {
		return result, signatureErr
	}

	var digest digestFile
	if err := json.Unmarshal(digestData, &digest); err != nil {
		return result, fmt.Errorf("parse %s: %w", DigestFileName, err)
	}
	if err := verifyDigest(dir, digest); err != nil {
		return result, err
	}
	result.Files = len(digest.Files)

	var signature signatureFile
	if err := json.Unmarshal(signatureData, &signature); err != nil {
		return result, fmt.Errorf("parse %s: %w", SignatureFileName, err)
	}
	if signature.Algorithm != "ed25519" {
		return result, fmt.Errorf("unsupported signature algorithm %q", signature.Algorithm)
	}
	publicKey, err := decodePublicKey(signature.PublicKey)
	if err != nil {
		return result, err
	}
	if opts.PublicKey != "" {
		expectedHex, _, err := LoadPublicKeyFile(opts.PublicKey)
		if err != nil {
			return result, err
		}
		if !strings.EqualFold(expectedHex, signature.PublicKey) {
			return result, fmt.Errorf("signature public key does not match %s", opts.PublicKey)
		}
	}
	signatureBytes, err := hex.DecodeString(signature.Signature)
	if err != nil {
		return result, fmt.Errorf("invalid signature hex: %w", err)
	}
	if !ed25519.Verify(publicKey, digestData, signatureBytes) {
		return result, fmt.Errorf("signature verification failed")
	}
	result.Signed = true
	result.Verified = true
	result.PublicKey = signature.PublicKey
	result.KeyID = signature.KeyID
	return result, nil
}

func LoadPublicKeyFile(path string) (string, ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	var key keyFile
	if err := json.Unmarshal(data, &key); err != nil {
		return "", nil, err
	}
	if key.Algorithm != "ed25519" {
		return "", nil, fmt.Errorf("unsupported key algorithm %q", key.Algorithm)
	}
	publicKey, err := decodePublicKey(key.PublicKey)
	if err != nil {
		return "", nil, err
	}
	return key.PublicKey, publicKey, nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var key keyFile
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, "", err
	}
	if key.Algorithm != "ed25519" {
		return nil, "", fmt.Errorf("unsupported key algorithm %q", key.Algorithm)
	}
	if key.PrivateKey == "" {
		return nil, "", fmt.Errorf("key file %s does not contain private_key", path)
	}
	privateKeyBytes, err := hex.DecodeString(key.PrivateKey)
	if err != nil {
		return nil, "", fmt.Errorf("invalid private_key hex: %w", err)
	}
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return nil, "", fmt.Errorf("invalid ed25519 private key length %d", len(privateKeyBytes))
	}
	publicKey, err := decodePublicKey(key.PublicKey)
	if err != nil {
		return nil, "", err
	}
	privateKey := ed25519.PrivateKey(privateKeyBytes)
	if !privateKey.Public().(ed25519.PublicKey).Equal(publicKey) {
		return nil, "", fmt.Errorf("private_key does not match public_key")
	}
	return privateKey, key.PublicKey, nil
}

func decodePublicKey(value string) (ed25519.PublicKey, error) {
	publicKeyBytes, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid public_key hex: %w", err)
	}
	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key length %d", len(publicKeyBytes))
	}
	return ed25519.PublicKey(publicKeyBytes), nil
}

func digestForEntries(entries map[string][]byte) digestFile {
	names := make([]string, 0, len(entries))
	for name := range entries {
		if ignoredIntegrityFile(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	digest := digestFile{Version: 1, DigestAlgorithm: "sha256"}
	for _, name := range names {
		sum := sha256.Sum256(entries[name])
		digest.Files = append(digest.Files, fileDigestEntry{Path: name, SHA256: hex.EncodeToString(sum[:])})
	}
	return digest
}

func verifyDigest(dir string, digest digestFile) error {
	if digest.Version != 1 {
		return fmt.Errorf("unsupported digest version %d", digest.Version)
	}
	if digest.DigestAlgorithm != "sha256" {
		return fmt.Errorf("unsupported digest algorithm %q", digest.DigestAlgorithm)
	}
	seen := map[string]bool{}
	for _, entry := range digest.Files {
		name := filepath.Clean(entry.Path)
		if name != entry.Path || filepath.IsAbs(entry.Path) || strings.HasPrefix(name, "..") {
			return fmt.Errorf("unsafe digest path %q", entry.Path)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), entry.SHA256) {
			return fmt.Errorf("digest mismatch for %s", entry.Path)
		}
		seen[name] = true
	}
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "wazero" || strings.HasPrefix(rel, "wazero/") {
				return filepath.SkipDir
			}
			return nil
		}
		if ignoredIntegrityFile(rel) || rel == "plugin.json" {
			return nil
		}
		if !seen[rel] {
			return fmt.Errorf("unsigned extra file %s", rel)
		}
		return nil
	})
}

func ignoredIntegrityFile(name string) bool {
	return name == DigestFileName || name == SignatureFileName
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

func keyID(publicHex string) string {
	publicBytes, err := hex.DecodeString(publicHex)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(publicBytes)
	return "ed25519:" + hex.EncodeToString(sum[:8])
}
