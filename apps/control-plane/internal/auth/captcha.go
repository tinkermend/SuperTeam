package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math/big"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

const (
	captchaDigits  = "23456789"
	captchaLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	captchaChars   = captchaDigits + captchaLetters
)

var errCaptchaRepositoryNotImplemented = errors.New("captcha repository not implemented")

func (s *Service) CreateCaptcha(ctx context.Context, clientIP, userAgent string) (*CaptchaChallenge, error) {
	now := s.now().UTC()
	if err := s.repo.DeleteExpiredCaptchaChallenges(ctx, now); err != nil {
		log.Printf("auth captcha expired cleanup skipped: err=%v", err)
	}
	code, err := generateCaptchaCode()
	if err != nil {
		return nil, err
	}
	imageDataURL, err := renderCaptchaImageDataURL(code)
	if err != nil {
		return nil, err
	}

	id := uuid.New()
	expiresAt := now.Add(s.captchaTTL)
	record, err := s.repo.CreateCaptchaChallenge(ctx, CreateCaptchaChallengeParams{
		ID:         id,
		TenantID:   uuid.MustParse(DefaultTenantID),
		AnswerHash: s.hashCaptchaAnswer(id.String(), code),
		ExpiresAt:  expiresAt,
		ClientIP:   strings.TrimSpace(clientIP),
		UserAgent:  strings.TrimSpace(userAgent),
	})
	if err != nil {
		return nil, err
	}
	return &CaptchaChallenge{
		ID:           record.ID,
		ImageDataURL: imageDataURL,
		ExpiresAt:    record.ExpiresAt,
	}, nil
}

func (s *Service) ValidateAndConsumeCaptcha(ctx context.Context, id uuid.UUID, answer, username, clientIP, userAgent string) error {
	answer = normalizeCaptchaAnswer(answer)
	if id == uuid.Nil || len(answer) != 4 {
		_ = s.recordCaptchaFailure(ctx, username, clientIP, userAgent, LoginFailureCaptchaInvalid)
		return ErrCaptchaInvalid
	}

	var resultErr error
	var failureReason string
	if err := s.repo.WithTransaction(ctx, func(repo Repository) error {
		record, err := repo.GetCaptchaChallengeForUpdate(ctx, id)
		if err != nil {
			if errors.Is(err, ErrCaptchaInvalid) {
				return ErrCaptchaInvalid
			}
			return err
		}
		if record.UsedAt != nil {
			failureReason = LoginFailureCaptchaInvalid
			return ErrCaptchaUsed
		}
		now := s.now().UTC()
		if !record.ExpiresAt.After(now) {
			if err := repo.ConsumeCaptchaChallenge(ctx, record.ID, now); err != nil && !errors.Is(err, ErrCaptchaUsed) {
				return err
			}
			failureReason = LoginFailureCaptchaExpired
			resultErr = ErrCaptchaExpired
			return nil
		}

		wantHash := s.hashCaptchaAnswer(record.ID.String(), answer)
		if subtle.ConstantTimeCompare([]byte(record.AnswerHash), []byte(wantHash)) != 1 {
			if err := repo.ConsumeCaptchaChallenge(ctx, record.ID, now); err != nil && !errors.Is(err, ErrCaptchaUsed) {
				return err
			}
			failureReason = LoginFailureCaptchaInvalid
			resultErr = ErrCaptchaInvalid
			return nil
		}
		return repo.ConsumeCaptchaChallenge(ctx, record.ID, now)
	}); err != nil {
		if errors.Is(err, ErrCaptchaInvalid) || errors.Is(err, ErrCaptchaUsed) {
			failureReason = LoginFailureCaptchaInvalid
			_ = s.recordCaptchaFailure(ctx, username, clientIP, userAgent, failureReason)
		}
		return err
	}
	if failureReason != "" {
		_ = s.recordCaptchaFailure(ctx, username, clientIP, userAgent, failureReason)
	}
	return resultErr
}

func (s *Service) DeleteExpiredCaptchaChallenges(ctx context.Context, before time.Time) error {
	return s.repo.DeleteExpiredCaptchaChallenges(ctx, before)
}

func (s *Service) hashCaptchaAnswer(id, answer string) string {
	mac := hmac.New(sha256.New, s.captchaSecret)
	mac.Write([]byte(strings.TrimSpace(id)))
	mac.Write([]byte{0})
	mac.Write([]byte(normalizeCaptchaAnswer(answer)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) recordCaptchaFailure(ctx context.Context, username, clientIP, userAgent, reason string) error {
	return s.repo.CreateLoginLog(ctx, CreateLoginLogParams{
		EventType:     LoginEventFailed,
		Username:      strings.TrimSpace(username),
		ClientIP:      strings.TrimSpace(clientIP),
		UserAgent:     strings.TrimSpace(userAgent),
		Result:        LoginResultFailed,
		FailureReason: reason,
	})
}

func normalizeCaptchaAnswer(answer string) string {
	return strings.ToUpper(strings.TrimSpace(answer))
}

func generateCaptchaCode() (string, error) {
	digit, err := randomChar(captchaDigits)
	if err != nil {
		return "", err
	}
	letter, err := randomChar(captchaLetters)
	if err != nil {
		return "", err
	}
	chars := []byte{digit, letter}
	for len(chars) < 4 {
		ch, err := randomChar(captchaChars)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}
	for i := len(chars) - 1; i > 0; i-- {
		j, err := randomInt(i + 1)
		if err != nil {
			return "", err
		}
		chars[i], chars[j] = chars[j], chars[i]
	}
	return string(chars), nil
}

func randomChar(chars string) (byte, error) {
	i, err := randomInt(len(chars))
	if err != nil {
		return 0, err
	}
	return chars[i], nil
}

func randomInt(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func captchaHasDigit(code string) bool {
	for _, ch := range code {
		if unicode.IsDigit(ch) {
			return true
		}
	}
	return false
}

func captchaHasLetter(code string) bool {
	for _, ch := range code {
		if unicode.IsLetter(ch) {
			return true
		}
	}
	return false
}

func renderCaptchaImageDataURL(code string) (string, error) {
	img := image.NewRGBA(image.Rect(0, 0, 140, 48))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 248, G: 250, B: 252, A: 255}}, image.Point{}, draw.Src)
	for x := 0; x < img.Bounds().Dx(); x += 12 {
		draw.Draw(img, image.Rect(x, 0, x+1, img.Bounds().Dy()), &image.Uniform{C: color.RGBA{R: 226, G: 232, B: 240, A: 255}}, image.Point{}, draw.Src)
	}
	for i, ch := range code {
		drawCaptchaGlyph(img, ch, 20+i*28, 10)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func drawCaptchaGlyph(img *image.RGBA, ch rune, x, y int) {
	rows := captchaGlyphs[ch]
	ink := &image.Uniform{C: color.RGBA{R: 30, G: 41, B: 59, A: 255}}
	for row, pattern := range rows {
		for col, pixel := range pattern {
			if pixel != '1' {
				continue
			}
			draw.Draw(img, image.Rect(x+col*3, y+row*4, x+col*3+3, y+row*4+4), ink, image.Point{}, draw.Src)
		}
	}
}

var captchaGlyphs = map[rune][]string{
	'2': {"11110", "00001", "00001", "11110", "10000", "10000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"10010", "10010", "10010", "11111", "00010", "00010", "00010"},
	'5': {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6': {"01111", "10000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00001", "11110"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01111", "10000", "10000", "10011", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'J': {"00111", "00010", "00010", "00010", "10010", "10010", "01100"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "10101", "11011", "10001"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
}

func (r *PgRepository) CreateCaptchaChallenge(ctx context.Context, params CreateCaptchaChallengeParams) (*CaptchaChallengeRecord, error) {
	return nil, errCaptchaRepositoryNotImplemented
}

func (r *PgRepository) GetCaptchaChallengeForUpdate(ctx context.Context, id uuid.UUID) (*CaptchaChallengeRecord, error) {
	return nil, errCaptchaRepositoryNotImplemented
}

func (r *PgRepository) ConsumeCaptchaChallenge(ctx context.Context, id uuid.UUID, usedAt time.Time) error {
	return errCaptchaRepositoryNotImplemented
}

func (r *PgRepository) DeleteExpiredCaptchaChallenges(ctx context.Context, before time.Time) error {
	return errCaptchaRepositoryNotImplemented
}
