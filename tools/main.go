package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: enc <PLAINTEXT_TOKEN>")
		os.Exit(2)
	}
	plaintext := []byte(os.Args[1])

	// 这里的 secret 需要你自己生成一个随机长字符串，然后硬编码到主程序里同样的值
	secret := []byte("CNPAI")
	key := sha256.Sum256(secret)

	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	out := append(nonce, ciphertext...)
	fmt.Println("enc:" + base64.RawStdEncoding.EncodeToString(out))
}
