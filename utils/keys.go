package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// GeneratePrivateKey generate an rsa key (actually used from the sshd server)
func GeneratePrivateKey(keyPath *string) {
	path, _ := ExpandUserHome(*keyPath)
	bitSize := 4096
	privateKey, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		log.Println(err)
	}

	// Validate Private Key
	err = privateKey.Validate()
	if err != nil {
		log.Println(err)
	}

	log.Println("private Key generated")

	privDER := x509.MarshalPKCS1PrivateKey(privateKey)

	// pem.Block
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}

	// Private key in PEM format
	privatePEM := pem.EncodeToMemory(&privBlock)
	if err := ioutil.WriteFile(path, privatePEM, 0600); err != nil {
		log.Println(err)
	}

	log.Printf("key saved to: %s", path)
}

// PublicKeyFile reads a public key file and loads the keys to
// an ssh.PublicKeys object
func PublicKeyFile(file string) ssh.AuthMethod {
	path, _ := ExpandUserHome(file)

	usr, _ := user.Current()
	// no path is set, try with a reasonable default
	if path == "" {
		path = filepath.Join(usr.HomeDir, ".ssh", "id_rsa")
	}

	buffer, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalln(fmt.Sprintf("cannot read SSH public key file %s", path))
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		log.Fatalln(fmt.Sprintf("cannot parse SSH public key file %s", file))
		return nil
	}
	return ssh.PublicKeys(key)
}

// AddHostKeyToKnownHosts updates user known_hosts file adding the host key
func AddHostKeyToKnownHosts(host string, key ssh.PublicKey) error {
	// add host key if host is not found in known_hosts, error object is return, if nil then connection proceeds,
	// if not nil then connection stops.
	var err error
	usr, err := user.Current()
	if err != nil {
		log.Fatalf("could not obtain user home directory :%v", err)
	}
	knownHostFile := filepath.Join(usr.HomeDir, ".ssh", "known_hosts")

	f, fErr := os.OpenFile(knownHostFile, os.O_APPEND|os.O_WRONLY, 0600)
	if fErr != nil {
		return fErr
	}
	defer f.Close()

	knownHosts := knownhosts.Normalize(host)
	out := fmt.Sprintf("%s\n", knownhosts.Line([]string{knownHosts}, key))
	_, fileErr := f.WriteString(out)
	return fileErr
}

// SerilizeKey converts an ssh.PublicKey to printable bas64 string
func SerializeKey(k ssh.PublicKey) string {
	return k.Type() + " " + base64.StdEncoding.EncodeToString(k.Marshal())
}
