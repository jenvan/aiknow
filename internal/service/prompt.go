package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/jenvan/aiknow/internal/model"
	"github.com/jenvan/aiknow/internal/repository"
)

// PromptKeyAnalyze 分析提示词在 settings 表中的键（AGENT.md：v1 仅此一个模板）
const PromptKeyAnalyze = "prompt.analyze"

// PromptService 提示词服务。
//
// 模板存 settings 表，缺失时回落到 L2 内置默认值。
type PromptService struct {
	settingRepo  repository.SettingRepository
	articleRepo  repository.ArticleRepository
	categoryRepo repository.CategoryRepository
	llmProvider  promptLLM
}

// promptLLM 只取 LLMProvider 中试跑所需的方法，便于单测替身
type promptLLM interface {
	GenerateJSON(ctx context.Context, prompt, schema string) (json.RawMessage, error)
}

// NewPromptService 创建提示词服务。
//
// 各依赖均可为 nil：缺 settingRepo 则只读默认模板且不可保存，
// 缺 llmProvider 则试跑只渲染不调用模型。
func NewPromptService(
	settingRepo repository.SettingRepository,
	articleRepo repository.ArticleRepository,
	categoryRepo repository.CategoryRepository,
	llmProvider promptLLM,
) *PromptService {
	return &PromptService{
		settingRepo:  settingRepo,
		articleRepo:  articleRepo,
		categoryRepo: categoryRepo,
		llmProvider:  llmProvider,
	}
}

// promptVars 模板可用变量（AGENT.md 规定的三个）
type promptVars struct {
	Title      string
	Content    string
	Categories string
}

// maxPromptContentRunes 正文注入模板前的截断长度（AGENT.md：前 8000 字）
const maxPromptContentRunes = 8000

// GetPrompt 获取提示词模板。
//
// 优先取库中已保存的值，未保存则返回 L2 内置默认值——
// 此前该方法返回一段硬编码的残缺字符串（只有开头一句、不含任何模板变量），
// 与流水线实际使用的模板不是同一份。
func (s *PromptService) GetPrompt(ctx context.Context, key string) (string, error) {
	if key != PromptKeyAnalyze {
		return "", fmt.Errorf("未知的提示词: %s（v1 仅支持 %s）", key, PromptKeyAnalyze)
	}

	if s.settingRepo != nil {
		setting, err := s.settingRepo.GetByKey(ctx, key)
		if err != nil {
			return "", fmt.Errorf("读取提示词失败: %w", err)
		}
		if setting != nil {
			var stored string
			if err := json.Unmarshal(setting.Value, &stored); err != nil {
				return "", fmt.Errorf("提示词存储格式异常（应为 JSON 字符串）: %w", err)
			}
			if stored != "" {
				return stored, nil
			}
		}
	}

	return model.DefaultL2Config().Prompt.Analyze, nil
}

// ValidatePrompt 校验模板：可编译、含必需变量、含 "json" 字样。
//
// 三项缺一不可：Content/Categories 缺失会让 LLM 拿不到正文或候选分类，
// 而 Qwen 的 JSON 模式要求提示词中出现 "json" 字样。
func ValidatePrompt(prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("提示词不能为空")
	}
	if !strings.Contains(prompt, "{{.Content}}") {
		return fmt.Errorf("提示词必须包含 {{.Content}}")
	}
	if !strings.Contains(prompt, "{{.Categories}}") {
		return fmt.Errorf("提示词必须包含 {{.Categories}}")
	}
	if !strings.Contains(strings.ToLower(prompt), "json") {
		return fmt.Errorf("提示词必须包含 \"json\" 字样（LLM JSON 模式要求）")
	}
	if _, err := template.New("prompt").Parse(prompt); err != nil {
		return fmt.Errorf("模板语有误: %w", err)
	}
	return nil
}

// UpdatePrompt 保存提示词模板。
//
// 此前校验通过后直接 return nil，修改被静默丢弃——调用方收到成功响应，
// 但下次读取仍是旧值。
func (s *PromptService) UpdatePrompt(ctx context.Context, key, prompt string) error {
	if key != PromptKeyAnalyze {
		return fmt.Errorf("未知的提示词: %s（v1 仅支持 %s）", key, PromptKeyAnalyze)
	}
	if err := ValidatePrompt(prompt); err != nil {
		return err
	}
	if s.settingRepo == nil {
		return fmt.Errorf("配置仓库未接线，无法保存提示词")
	}

	value, err := json.Marshal(prompt)
	if err != nil {
		return fmt.Errorf("序列化提示词失败: %w", err)
	}

	if err := s.settingRepo.Upsert(ctx, &model.Setting{
		Key:   key,
		Value: value,
	}); err != nil {
		return fmt.Errorf("保存提示词失败: %w", err)
	}
	return nil
}

// PromptResult 提示词测试结果
type PromptResult struct {
	RenderedPrompt  string `json:"rendered_prompt"`
	LLMResponse     string `json:"llm_response,omitempty"`
	ValidationError string `json:"validation_error,omitempty"`
}

// TestPrompt 用未保存的模板试跑指定文章：展示渲染结果、模型原始返回与校验结果，
// 不写入任何数据（AGENT.md 明确要求）。
func (s *PromptService) TestPrompt(ctx context.Context, promptTemplate, articleID string) (*PromptResult, error) {
	result := &PromptResult{}

	// 校验不通过也继续渲染：调试时需要看到渲染结果才能定位问题
	if err := ValidatePrompt(promptTemplate); err != nil {
		result.ValidationError = err.Error()
	}

	tmpl, err := template.New("prompt").Parse(promptTemplate)
	if err != nil {
		// 语法错误无法渲染，原样回显模板
		result.RenderedPrompt = promptTemplate
		if result.ValidationError == "" {
			result.ValidationError = err.Error()
		}
		return result, nil
	}

	vars, err := s.buildVars(ctx, articleID)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("渲染模板失败: %w", err)
	}
	result.RenderedPrompt = buf.String()

	// 未指定文章或未接 LLM 时只做渲染
	if articleID == "" || s.llmProvider == nil {
		return result, nil
	}

	raw, err := s.llmProvider.GenerateJSON(ctx, result.RenderedPrompt, "")
	if err != nil {
		// 调用失败属试跑的有效结果之一，记入结果而非中断
		result.ValidationError = "LLM 调用失败: " + err.Error()
		return result, nil
	}
	result.LLMResponse = string(raw)

	// 试跑同时校验返回是否为合法 JSON，省得用户自己比对
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		result.ValidationError = "模型返回不是合法 JSON: " + err.Error()
	}

	return result, nil
}

// buildVars 组装模板变量。articleID 为空时用占位示例，便于纯语法试渲染。
func (s *PromptService) buildVars(ctx context.Context, articleID string) (*promptVars, error) {
	vars := &promptVars{
		Title:   "（示例标题）",
		Content: "（示例正文）",
	}

	if s.categoryRepo != nil {
		cats, err := s.categoryRepo.ListAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("读取分类列表失败: %w", err)
		}
		names := make([]string, 0, len(cats))
		for _, c := range cats {
			names = append(names, c.Name)
		}
		vars.Categories = strings.Join(names, ", ")
	}

	if articleID == "" || s.articleRepo == nil {
		return vars, nil
	}

	article, err := s.articleRepo.GetByID(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("读取文章失败: %w", err)
	}
	if article == nil {
		return nil, fmt.Errorf("文章不存在: %s", articleID)
	}

	vars.Title = article.Title
	vars.Content = truncateRunes(article.Content, maxPromptContentRunes)
	return vars, nil
}

// truncateRunes 按字符（非字节）截断，避免切断多字节汉字
func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
