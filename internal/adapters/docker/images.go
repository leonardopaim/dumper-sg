package docker

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Download missing images before the bounded container creation step. On a new
// installation a pull can take minutes; cancellation here creates no container.
func (e *Executor) ensureImage(ctx context.Context, image string, emit func(string, string)) error {
	if strings.TrimSpace(image) == "" || strings.HasPrefix(image, "-") || strings.ContainsAny(image, "\x00\r\n") {
		return fmt.Errorf("imagem Docker inválida")
	}
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	_, err := e.invoke(probe, "image", "inspect", "--format", "{{.Id}}", image)
	cancel()
	if err == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if emit != nil {
		emit("info", "Primeiro uso: baixando imagem "+image+". Aguarde; isso pode levar alguns minutos.")
	}
	pull, cancelPull := context.WithTimeout(ctx, 15*time.Minute)
	defer cancelPull()
	cmd := e.command(pull, "pull", image)
	out := &lineWriter{level: "info", emit: emit}
	stderr := &lineWriter{level: "error", emit: emit}
	cmd.Stdout, cmd.Stderr = out, stderr
	cmd.WaitDelay = 3 * time.Second
	err = cmd.Run()
	out.flush()
	stderr.flush()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := pull.Err(); err != nil {
		return fmt.Errorf("download da imagem excedeu 15 minutos: %w", err)
	}
	if err != nil {
		return fmt.Errorf("não foi possível baixar %s; verifique a internet e o Docker: %w", image, err)
	}
	if emit != nil {
		emit("info", "Imagem disponível: "+image)
	}
	return nil
}
