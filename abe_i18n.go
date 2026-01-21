package abe

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/viper"
	"golang.org/x/text/language"
)

// newI18nBundle 初始化并加载 YAML 消息文件，仅在启动阶段调用
func newI18nBundle(config *viper.Viper, logger *slog.Logger) *i18n.Bundle {
	if config == nil {
		return nil
	}

	base := config.GetString("i18n.default_language")
	if base == "" {
		base = "en"
	}
	bundle := i18n.NewBundle(language.MustParse(base))
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	paths := config.GetStringSlice("i18n.message_paths")
	for _, dir := range paths {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if logger != nil {
				logger.Warn("加载翻译目录失败", "dir", dir, "error", err)
			}
			continue
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			name := ent.Name()
			if !strings.HasPrefix(name, "active.") || !strings.HasSuffix(name, ".yaml") {
				continue
			}
			fp := filepath.Join(dir, name)
			if _, err := bundle.LoadMessageFile(fp); err != nil {
				if logger != nil {
					logger.Warn("加载翻译文件失败", "file", fp, "error", err)
				}
			} else {
				if logger != nil {
					logger.Info("已加载翻译文件", "file", fp)
				}
			}
		}
	}

	return bundle
}
