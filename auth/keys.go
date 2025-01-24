package auth

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"math/big"
)

// GenerateKey creates a deterministic SSH private key from a seed value
func GenerateKey(seed string) ([]byte, error) {
	// Generate a deterministic seed based on the input
	hash := sha256.Sum256([]byte(seed))
	seedBytes := hash[:]

	// Use the seed to initialize the random number generator
	reader := &seedReader{seed: seedBytes}

	// Generate RSA key with deterministic parameters
	key := &rsa.PrivateKey{
		PublicKey: rsa.PublicKey{
			N: new(big.Int),
			E: 65537, // Standard RSA exponent
		},
		D: new(big.Int),
	}

	// Generate deterministic prime factors
	p := new(big.Int)
	q := new(big.Int)

	// Use the seed to generate prime factors
	pBytes := make([]byte, 128) // 1024 bits
	qBytes := make([]byte, 128) // 1024 bits
	reader.Read(pBytes)
	reader.Read(qBytes)

	// Ensure the most significant bit is set for both primes
	pBytes[0] |= 0x80
	qBytes[0] |= 0x80

	// Convert to big integers and ensure they are prime
	p.SetBytes(pBytes)
	q.SetBytes(qBytes)

	// Find the next prime numbers by incrementing until we find primes
	for !p.ProbablyPrime(20) {
		p.Add(p, big.NewInt(1))
	}
	for !q.ProbablyPrime(20) {
		q.Add(q, big.NewInt(1))
	}

	// Calculate RSA parameters
	key.PublicKey.N.Mul(p, q)
	phi := new(big.Int).Mul(
		new(big.Int).Sub(p, big.NewInt(1)),
		new(big.Int).Sub(q, big.NewInt(1)),
	)

	// Calculate private exponent
	key.D.ModInverse(big.NewInt(65537), phi)

	// Additional RSA parameters
	key.Primes = []*big.Int{p, q}
	key.Precomputed = rsa.PrecomputedValues{
		Dp:        new(big.Int).Mod(key.D, new(big.Int).Sub(p, big.NewInt(1))),
		Dq:        new(big.Int).Mod(key.D, new(big.Int).Sub(q, big.NewInt(1))),
		Qinv:      new(big.Int).ModInverse(q, p),
		CRTValues: []rsa.CRTValue{},
	}

	// Create a deterministic PEM block
	derBytes := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: derBytes,
	}

	return pem.EncodeToMemory(block), nil
}

// seedReader implements io.Reader using a seed for deterministic randomness
type seedReader struct {
	seed   []byte
	offset int
}

func (r *seedReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.seed) {
		// Generate more deterministic bytes using the seed
		hash := sha256.New()
		hash.Write(r.seed)
		hash.Write([]byte{byte(r.offset)})
		r.seed = hash.Sum(nil)
		r.offset = 0
	}

	n = copy(p, r.seed[r.offset:])
	r.offset += n
	return n, nil
}
