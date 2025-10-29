package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrNil = errors.New("redis: nil")

type RedisClient struct {
	mu   sync.Mutex
	addr string
	conn net.Conn
	rw   *bufio.ReadWriter
}

func NewRedisClient(addr string) *RedisClient {
	return &RedisClient{addr: addr}
}

func (c *RedisClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *RedisClient) HSet(ctx context.Context, key string, fields map[string]interface{}) error {
	args := []any{key}
	for k, v := range fields {
		args = append(args, k, toString(v))
	}
	_, err := c.do(ctx, "HSET", args...)
	return err
}

func (c *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	v, err := c.do(ctx, "HGETALL", key)
	if err != nil {
		return nil, err
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("HGETALL: unexpected type %T", v)
	}
	if len(arr)%2 != 0 {
		return nil, fmt.Errorf("HGETALL: expected even array length, got %d", len(arr))
	}
	out := make(map[string]string, len(arr)/2)
	for i := 0; i < len(arr); i += 2 {
		kb, ok1 := arr[i].([]byte)
		vb, ok2 := arr[i+1].([]byte)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("HGETALL: expected bulk strings")
		}
		out[string(kb)] = string(vb)
	}
	return out, nil
}

func (c *RedisClient) ZAdd(ctx context.Context, key string, score float64, member string) error {
	_, err := c.do(ctx, "ZADD", key, formatFloat(score), member)
	return err
}

func (c *RedisClient) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	v, err := c.do(ctx, "ZREVRANGE", key, strconv.FormatInt(start, 10), strconv.FormatInt(stop, 10))
	if err != nil {
		return nil, err
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("ZREVRANGE: unexpected type %T", v)
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		b, ok := it.([]byte)
		if !ok {
			return nil, fmt.Errorf("ZREVRANGE: expected bulk string, got %T", it)
		}
		out = append(out, string(b))
	}
	return out, nil
}

func (c *RedisClient) Set(ctx context.Context, key string, value []byte, ttlSeconds int) error {
	args := []any{key, value}
	if ttlSeconds > 0 {
		args = append(args, "EX", strconv.Itoa(ttlSeconds))
	}
	_, err := c.do(ctx, "SET", args...)
	return err
}

func (c *RedisClient) GetBytes(ctx context.Context, key string) ([]byte, error) {
	v, err := c.do(ctx, "GET", key)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrNil
	}
	b, ok := v.([]byte)
	if !ok {
		return nil, fmt.Errorf("GET: unexpected type %T", v)
	}
	return b, nil
}

func (c *RedisClient) GetString(ctx context.Context, key string) (string, error) {
	b, err := c.GetBytes(ctx, key)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *RedisClient) Exists(ctx context.Context, key string) (int64, error) {
	v, err := c.do(ctx, "EXISTS", key)
	if err != nil {
		return 0, err
	}
	n, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("EXISTS: unexpected type %T", v)
	}
	return n, nil
}

func (c *RedisClient) do(ctx context.Context, cmd string, args ...any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	sendOnce := func() (any, error) {
		if c.conn == nil {
			if err := c.connectLocked(ctx); err != nil {
				return nil, err
			}
		}
		if dl, ok := ctx.Deadline(); ok {
			_ = c.conn.SetDeadline(dl)
		} else {
			_ = c.conn.SetDeadline(time.Now().Add(5 * time.Second))
		}
		defer c.conn.SetDeadline(time.Time{})
		if err := writeArrayHeader(c.rw, 1+len(args)); err != nil {
			return nil, err
		}
		if err := writeBulk(c.rw, strings.ToUpper(cmd)); err != nil {
			return nil, err
		}
		for _, a := range args {
			switch v := a.(type) {
			case []byte:
				if err := writeBulkBytes(c.rw, v); err != nil {
					return nil, err
				}
			default:
				if err := writeBulk(c.rw, toString(v)); err != nil {
					return nil, err
				}
			}
		}
		if err := c.rw.Flush(); err != nil {
			return nil, err
		}
		return readResp(c.rw.Reader)
	}

	v, err := sendOnce()
	if !isRetryableNetErr(err) {
		return v, err
	}

	_ = c.resetLocked()
	if recErr := c.connectLocked(ctx); recErr != nil {
		if err == nil {
			err = recErr
		}
		return nil, err
	}
	return sendOnce()
}

func (c *RedisClient) connectLocked(ctx context.Context) error {
	timeout := 5 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if t := time.Until(dl); t > 0 {
			timeout = t
		}
	}
	d := &net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return err
	}
	c.conn = conn
	c.rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	return nil
}

func (c *RedisClient) resetLocked() error {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.rw = nil
	return nil
}

func isRetryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") {
		return true
	}
	return false
}

func writeArrayHeader(w *bufio.ReadWriter, n int) error {
	_, err := w.WriteString("*" + strconv.Itoa(n) + "\r\n")
	return err
}

func writeBulk(w *bufio.ReadWriter, s string) error {
	return writeBulkBytes(w, []byte(s))
}

func writeBulkBytes(w *bufio.ReadWriter, b []byte) error {
	if _, err := w.WriteString("$" + strconv.Itoa(len(b)) + "\r\n"); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err := w.WriteString("\r\n")
	return err
}

func readLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(s, "\n"), "\r"), nil
}

func readResp(r *bufio.Reader) (any, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if len(line) == 0 {
		return nil, errors.New("empty redis response")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, errors.New(line[1:])
	case ':':
		n, err := strconv.ParseInt(line[1:], 10, 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n == -1 {
			return nil, nil
		}
		buf := make([]byte, n)
		if _, err := ioReadFull(r, buf); err != nil {
			return nil, err
		}
		if _, err := r.ReadByte(); err != nil {
			return nil, err
		}
		if _, err := r.ReadByte(); err != nil {
			return nil, err
		}
		return buf, nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n == -1 {
			return nil, nil
		}
		arr := make([]any, 0, n)
		for i := 0; i < n; i++ {
			v, err := readResp(r)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("unknown RESP type: %q", line[0])
	}
}

func ioReadFull(r *bufio.Reader, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		k, err := r.Read(b[n:])
		n += k
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return formatFloat(t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprint(v)
	}
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
