package identity

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	temporaryPasswordLetters  = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	temporaryPasswordDigits   = "23456789"
	temporaryPasswordAlphabet = temporaryPasswordLetters + temporaryPasswordDigits
)

// GenerateTemporaryPassword creates a cryptographically random password that
// is guaranteed to satisfy the current policy for the target identity.
func GenerateTemporaryPassword(length int, username, email string) (string, error) {
	if length < PasswordMinLength || length > PasswordMaxLength {
		return "", fmt.Errorf("temporary password length must be between %d and %d", PasswordMinLength, PasswordMaxLength)
	}
	for attempt := 0; attempt < 32; attempt++ {
		password := make([]byte, length)
		var err error
		if password[0], err = randomPasswordCharacter(temporaryPasswordLetters); err != nil {
			return "", err
		}
		if password[1], err = randomPasswordCharacter(temporaryPasswordDigits); err != nil {
			return "", err
		}
		for index := 2; index < len(password); index++ {
			if password[index], err = randomPasswordCharacter(temporaryPasswordAlphabet); err != nil {
				return "", err
			}
		}
		if err := shufflePassword(password); err != nil {
			return "", err
		}
		candidate := string(password)
		if ValidatePassword(candidate, username, email) == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("generate temporary password satisfying identity policy")
}

func randomPasswordCharacter(alphabet string) (byte, error) {
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
	if err != nil {
		return 0, fmt.Errorf("generate temporary password: %w", err)
	}
	return alphabet[index.Int64()], nil
}

func shufflePassword(password []byte) error {
	for index := len(password) - 1; index > 0; index-- {
		other, err := rand.Int(rand.Reader, big.NewInt(int64(index+1)))
		if err != nil {
			return fmt.Errorf("shuffle temporary password: %w", err)
		}
		password[index], password[other.Int64()] = password[other.Int64()], password[index]
	}
	return nil
}
