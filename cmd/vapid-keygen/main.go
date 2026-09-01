package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func writePrivateKeyFile(path, privateKey string) (string, error) {
	absPath, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("resolve private key file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o700); err != nil {
		return "", fmt.Errorf("create private key directory: %w", err)
	}
	file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create private key file: %w", err)
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(absPath)
		}
	}()
	if _, err := file.WriteString(strings.TrimSpace(privateKey) + "\n"); err != nil {
		return "", fmt.Errorf("write private key file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close private key file: %w", err)
	}
	if err := os.Chmod(absPath, 0o600); err != nil {
		return "", fmt.Errorf("restrict private key file: %w", err)
	}
	keep = true
	return absPath, nil
}

func main() {
	privateKeyFile := flag.String("private-key-file", "", "write the private key to a new 0600 file instead of printing it")
	flag.Parse()

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatal("generate VAPID keys: ", err)
	}
	if strings.TrimSpace(*privateKeyFile) != "" {
		writtenPath, err := writePrivateKeyFile(*privateKeyFile, privateKey)
		if err != nil {
			log.Fatal("save VAPID private key: ", err)
		}
		fmt.Println("WEB_PUSH_VAPID_PUBLIC_KEY=" + publicKey)
		fmt.Println("WEB_PUSH_VAPID_PRIVATE_KEY_FILE=" + writtenPath)
		return
	}
	fmt.Println("WEB_PUSH_VAPID_PUBLIC_KEY=" + publicKey)
	fmt.Println("WEB_PUSH_VAPID_PRIVATE_KEY=" + privateKey)
}
