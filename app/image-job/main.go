package main

import (
	"bytes"
	"context"
	"flag"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	var (
		redisAddr = getenv("REDIS_ADDR", "redis:6379")
		imageID   = getenv("IMAGE_ID", "")
		effect    = strings.ToLower(getenv("EFFECT", ""))
	)
	flag.StringVar(&redisAddr, "redis", redisAddr, "Redis host:port")
	flag.StringVar(&imageID, "id", imageID, "Image ID (required)")
	flag.StringVar(&effect, "effect", effect, "Effect: grayscale | invert (required)")
	flag.Parse()

	if imageID == "" || effect == "" {
		log.Fatalf("[fatal] IMAGE_ID and EFFECT are required (got id=%q effect=%q)", imageID, effect)
	}
	if effect != "grayscale" && effect != "invert" {
		log.Fatalf("[fatal] unsupported effect %q (use: grayscale | invert)", effect)
	}

	rdb, err := NewRedisClient(redisAddr)
	if err != nil {
		log.Fatalf("[fatal] connect redis %s: %v", redisAddr, err)
	}
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	key := "image:" + imageID
	ctypeKey := "image:ctype:" + imageID

	srcBytes, err := rdb.GetBytes(ctx, key)
	if err != nil {
		log.Fatalf("[fatal] get %s: %v", key, err)
	}
	log.Printf("[info] loaded image bytes=%d", len(srcBytes))

	srcImg, format, err := image.Decode(bytes.NewReader(srcBytes))
	if err != nil {
		log.Fatalf("[fatal] decode: %v", err)
	}
	log.Printf("[info] decoded format=%s bounds=%v", format, srcImg.Bounds())

	var outImg image.Image
	switch effect {
	case "grayscale":
		outImg = toGrayscale(srcImg)
	case "invert":
		outImg = invertColors(srcImg)
	default:
		log.Fatalf("[fatal] unreachable effect %q", effect)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, outImg); err != nil {
		log.Fatalf("[fatal] png encode: %v", err)
	}

	if err := rdb.Set(ctx, key, buf.Bytes(), 0); err != nil {
		log.Fatalf("[fatal] set %s: %v", key, err)
	}
	if err := rdb.Set(ctx, ctypeKey, []byte("image/png"), 0); err != nil {
		log.Fatalf("[fatal] set %s: %v", ctypeKey, err)
	}

	_ = rdb.Set(ctx, "image:fx:"+imageID, []byte(effect), 0)

	log.Printf("[done] effect=%s wrote %d bytes to %s (ctype=image/png)", effect, buf.Len(), key)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
