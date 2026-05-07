     1|package service
     2|
     3|import (
     4|	"context"
     5|	"crypto/sha256"
     6|	"encoding/hex"
     7|	"encoding/json"
     8|	"fmt"
     9|	"os"
    10|	"path/filepath"
    11|	"regexp"
    12|	"strings"
    13|	"sync"
    14|	"time"
    15|
    16|	"github.com/Wei-Shaw/sub2api/internal/config"
    17|	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
    18|	"github.com/Wei-Shaw/sub2api/internal/pkg/modelmetadata"
    19|	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
    20|	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
    21|	"go.uber.org/zap"
    22|)
    23|
    24|var (
    25|	openAIModelDatePattern     = regexp.MustCompile(`-\d{8}$`)
    26|	openAIModelBasePattern     = regexp.MustCompile(`^(gpt-\d+(?:\.\d+)?)(?:-|$)`)
    27|	openAIGPT54FallbackPricing = &LiteLLMModelPricing{
    28|		InputCostPerToken:               2.5e-06, // $2.5 per MTok
    29|		OutputCostPerToken:              1.5e-05, // $15 per MTok
    30|		CacheReadInputTokenCost:         2.5e-07, // $0.25 per MTok
    31|		LongContextInputTokenThreshold:  272000,
    32|		LongContextInputCostMultiplier:  2.0,
    33|		LongContextOutputCostMultiplier: 1.5,
    34|		LiteLLMProvider:                 "openai",
    35|		Mode:                            "chat",
    36|		SupportsPromptCaching:           true,
    37|	}
    38|	openAIGPT54MiniFallbackPricing = &LiteLLMModelPricing{
    39|		InputCostPerToken:       7.5e-07,
    40|		OutputCostPerToken:      4.5e-06,
    41|		CacheReadInputTokenCost: 7.5e-08,
    42|		LiteLLMProvider:         "openai",
    43|		Mode:                    "chat",
    44|		SupportsPromptCaching:   true,
    45|	}
    46|	openAIGPT54NanoFallbackPricing = &LiteLLMModelPricing{
    47|		InputCostPerToken:       2e-07,
    48|		OutputCostPerToken:      1.25e-06,
    49|		CacheReadInputTokenCost: 2e-08,
    50|		LiteLLMProvider:         "openai",
    51|		Mode:                    "chat",
    52|		SupportsPromptCaching:   true,
    53|	}
    54|)
    55|
    56|// LiteLLMModelPricing LiteLLM价格数据结构
    57|// 只保留我们需要的字段，使用指针来处理可能缺失的值
type LiteLLMModelPricing struct {
	InputCostPerToken                   float64  `json:"input_cost_per_token"`
	InputCostPerTokenPriority           float64  `json:"input_cost_per_token_priority"`
	OutputCostPerToken                  float64  `json:"output_cost_per_token"`
	OutputCostPerTokenPriority          float64  `json:"output_cost_per_token_priority"`
	CacheCreationInputTokenCost         float64  `json:"cache_creation_input_token_cost"`
	CacheCreationInputTokenCostAbove1hr float64  `json:"cache_creation_input_token_cost_above_1hr"`
	CacheReadInputTokenCost             float64  `json:"cache_read_input_token_cost"`
	CacheReadInputTokenCostPriority     float64  `json:"cache_read_input_token_cost_priority"`
	LongContextInputTokenThreshold      int      `json:"long_context_input_token_threshold,omitempty"`
	LongContextInputCostMultiplier      float64  `json:"long_context_input_cost_multiplier,omitempty"`
	LongContextOutputCostMultiplier     float64  `json:"long_context_output_cost_multiplier,omitempty"`
	MaxInputTokens                      int      `json:"max_input_tokens,omitempty"`
	MaxOutputTokens                     int      `json:"max_output_tokens,omitempty"`
	MaxTokens                           int      `json:"max_tokens,omitempty"`
	SupportsServiceTier                 bool     `json:"supports_service_tier"`
	LiteLLMProvider                     string   `json:"litellm_provider"`
	Mode                                string   `json:"mode"`
	SupportsPromptCaching               bool     `json:"supports_prompt_caching"`
	SupportedModalities                 []string `json:"supported_modalities,omitempty"`
	SupportedOutputModalities           []string `json:"supported_output_modalities,omitempty"`
	SupportsPDFInput                    bool     `json:"supports_pdf_input"`
	OutputCostPerImage                  float64  `json:"output_cost_per_image"`       // 图片生成模型每张图片价格
	OutputCostPerImageToken             float64  `json:"output_cost_per_image_token"` // 图片输出 token 价格
}

    81|// PricingRemoteClient 远程价格数据获取接口
    82|type PricingRemoteClient interface {
    83|	FetchPricingJSON(ctx context.Context, url string) ([]byte, error)
    84|	FetchHashText(ctx context.Context, url string) (string, error)
    85|}
    86|
    87|// LiteLLMRawEntry 用于解析原始JSON数据
type LiteLLMRawEntry struct {
	InputCostPerToken                   *float64 `json:"input_cost_per_token"`
	InputCostPerTokenPriority           *float64 `json:"input_cost_per_token_priority"`
	OutputCostPerToken                  *float64 `json:"output_cost_per_token"`
	OutputCostPerTokenPriority          *float64 `json:"output_cost_per_token_priority"`
	CacheCreationInputTokenCost         *float64 `json:"cache_creation_input_token_cost"`
	CacheCreationInputTokenCostAbove1hr *float64 `json:"cache_creation_input_token_cost_above_1hr"`
	CacheReadInputTokenCost             *float64 `json:"cache_read_input_token_cost"`
	CacheReadInputTokenCostPriority     *float64 `json:"cache_read_input_token_cost_priority"`
	MaxInputTokens                      *int     `json:"max_input_tokens"`
	MaxOutputTokens                     *int     `json:"max_output_tokens"`
	MaxTokens                           *int     `json:"max_tokens"`
	SupportsServiceTier                 bool     `json:"supports_service_tier"`
	LiteLLMProvider                     string   `json:"litellm_provider"`
	Mode                                string   `json:"mode"`
	SupportsPromptCaching               bool     `json:"supports_prompt_caching"`
	SupportedModalities                 []string `json:"supported_modalities"`
	SupportedOutputModalities           []string `json:"supported_output_modalities"`
	SupportsPDFInput                    bool     `json:"supports_pdf_input"`
	OutputCostPerImage                  *float64 `json:"output_cost_per_image"`
	OutputCostPerImageToken             *float64 `json:"output_cost_per_image_token"`
}

   108|// PricingService 动态价格服务
   109|type PricingService struct {
   110|	cfg          *config.Config
   111|	remoteClient PricingRemoteClient
   112|	mu           sync.RWMutex
   113|	pricingData  map[string]*LiteLLMModelPricing
   114|	lastUpdated  time.Time
   115|	localHash    string
   116|
   117|	// 停止信号
   118|	stopCh chan struct{}
   119|	wg     sync.WaitGroup
   120|}
   121|
   122|// NewPricingService 创建价格服务
   123|func NewPricingService(cfg *config.Config, remoteClient PricingRemoteClient) *PricingService {
   124|	s := &PricingService{
   125|		cfg:          cfg,
   126|		remoteClient: remoteClient,
   127|		pricingData:  make(map[string]*LiteLLMModelPricing),
   128|		stopCh:       make(chan struct{}),
   129|	}
   130|	return s
   131|}
   132|
   133|// Initialize 初始化价格服务
   134|func (s *PricingService) Initialize() error {
   135|	// 确保数据目录存在
   136|	if err := os.MkdirAll(s.cfg.Pricing.DataDir, 0755); err != nil {
   137|		logger.LegacyPrintf("service.pricing", "[Pricing] Failed to create data directory: %v", err)
   138|	}
   139|
   140|	// 首次加载价格数据
   141|	if err := s.checkAndUpdatePricing(); err != nil {
   142|		logger.LegacyPrintf("service.pricing", "[Pricing] Initial load failed, using fallback: %v", err)
   143|		if err := s.useFallbackPricing(); err != nil {
   144|			return fmt.Errorf("failed to load pricing data: %w", err)
   145|		}
   146|	}
   147|
   148|	// 启动定时更新
   149|	s.startUpdateScheduler()
   150|
   151|	logger.LegacyPrintf("service.pricing", "[Pricing] Service initialized with %d models", len(s.pricingData))
   152|	return nil
   153|}
   154|
   155|// Stop 停止价格服务
   156|func (s *PricingService) Stop() {
   157|	close(s.stopCh)
   158|	s.wg.Wait()
   159|	logger.LegacyPrintf("service.pricing", "%s", "[Pricing] Service stopped")
   160|}
   161|
   162|// startUpdateScheduler 启动定时更新调度器
   163|func (s *PricingService) startUpdateScheduler() {
   164|	// 定期检查哈希更新
   165|	hashInterval := time.Duration(s.cfg.Pricing.HashCheckIntervalMinutes) * time.Minute
   166|	if hashInterval < time.Minute {
   167|		hashInterval = 10 * time.Minute
   168|	}
   169|
   170|	s.wg.Add(1)
   171|	go func() {
   172|		defer s.wg.Done()
   173|		ticker := time.NewTicker(hashInterval)
   174|		defer ticker.Stop()
   175|
   176|		for {
   177|			select {
   178|			case <-ticker.C:
   179|				if err := s.syncWithRemote(); err != nil {
   180|					logger.LegacyPrintf("service.pricing", "[Pricing] Sync failed: %v", err)
   181|				}
   182|			case <-s.stopCh:
   183|				return
   184|			}
   185|		}
   186|	}()
   187|
   188|	logger.LegacyPrintf("service.pricing", "[Pricing] Update scheduler started (check every %v)", hashInterval)
   189|}
   190|
   191|// checkAndUpdatePricing 检查并更新价格数据
   192|func (s *PricingService) checkAndUpdatePricing() error {
   193|	pricingFile := s.getPricingFilePath()
   194|
   195|	// 检查本地文件是否存在
   196|	if _, err := os.Stat(pricingFile); os.IsNotExist(err) {
   197|		logger.LegacyPrintf("service.pricing", "%s", "[Pricing] Local pricing file not found, downloading...")
   198|		return s.downloadPricingData()
   199|	}
   200|
   201|	// 先加载本地文件（确保服务可用），再检查是否需要更新
   202|	if err := s.loadPricingData(pricingFile); err != nil {
   203|		logger.LegacyPrintf("service.pricing", "[Pricing] Failed to load local file, downloading: %v", err)
   204|		return s.downloadPricingData()
   205|	}
   206|
   207|	// 如果配置了哈希URL，通过远程哈希检查是否有更新
   208|	if s.cfg.Pricing.HashURL != "" {
   209|		remoteHash, err := s.fetchRemoteHash()
   210|		if err != nil {
   211|			logger.LegacyPrintf("service.pricing", "[Pricing] Failed to fetch remote hash on startup: %v", err)
   212|			return nil // 已加载本地文件，哈希获取失败不影响启动
   213|		}
   214|
   215|		s.mu.RLock()
   216|		localHash := s.localHash
   217|		s.mu.RUnlock()
   218|
   219|		if localHash == "" || remoteHash != localHash {
   220|			logger.LegacyPrintf("service.pricing", "[Pricing] Remote hash differs on startup (local=%s remote=%s), downloading...",
   221|				localHash[:min(8, len(localHash))], remoteHash[:min(8, len(remoteHash))])
   222|			if err := s.downloadPricingData(); err != nil {
   223|				logger.LegacyPrintf("service.pricing", "[Pricing] Download failed, using existing file: %v", err)
   224|			}
   225|		}
   226|		return nil
   227|	}
   228|
   229|	// 没有哈希URL时，基于文件年龄检查
   230|	info, err := os.Stat(pricingFile)
   231|	if err != nil {
   232|		return nil // 已加载本地文件
   233|	}
   234|
   235|	fileAge := time.Since(info.ModTime())
   236|	maxAge := time.Duration(s.cfg.Pricing.UpdateIntervalHours) * time.Hour
   237|
   238|	if fileAge > maxAge {
   239|		logger.LegacyPrintf("service.pricing", "[Pricing] Local file is %v old, updating...", fileAge.Round(time.Hour))
   240|		if err := s.downloadPricingData(); err != nil {
   241|			logger.LegacyPrintf("service.pricing", "[Pricing] Download failed, using existing file: %v", err)
   242|		}
   243|	}
   244|
   245|	return nil
   246|}
   247|
   248|// syncWithRemote 与远程同步（基于哈希校验）
   249|func (s *PricingService) syncWithRemote() error {
   250|	// 如果配置了哈希URL，从远程获取哈希进行比对
   251|	if s.cfg.Pricing.HashURL != "" {
   252|		remoteHash, err := s.fetchRemoteHash()
   253|		if err != nil {
   254|			logger.LegacyPrintf("service.pricing", "[Pricing] Failed to fetch remote hash: %v", err)
   255|			return nil // 哈希获取失败不影响正常使用
   256|		}
   257|
   258|		s.mu.RLock()
   259|		localHash := s.localHash
   260|		s.mu.RUnlock()
   261|
   262|		if localHash == "" || remoteHash != localHash {
   263|			logger.LegacyPrintf("service.pricing", "[Pricing] Remote hash differs (local=%s remote=%s), downloading new version...",
   264|				localHash[:min(8, len(localHash))], remoteHash[:min(8, len(remoteHash))])
   265|			return s.downloadPricingData()
   266|		}
   267|		logger.LegacyPrintf("service.pricing", "%s", "[Pricing] Hash check passed, no update needed")
   268|		return nil
   269|	}
   270|
   271|	// 没有哈希URL时，基于时间检查
   272|	pricingFile := s.getPricingFilePath()
   273|	info, err := os.Stat(pricingFile)
   274|	if err != nil {
   275|		return s.downloadPricingData()
   276|	}
   277|
   278|	fileAge := time.Since(info.ModTime())
   279|	maxAge := time.Duration(s.cfg.Pricing.UpdateIntervalHours) * time.Hour
   280|
   281|	if fileAge > maxAge {
   282|		logger.LegacyPrintf("service.pricing", "[Pricing] File is %v old, downloading...", fileAge.Round(time.Hour))
   283|		return s.downloadPricingData()
   284|	}
   285|
   286|	return nil
   287|}
   288|
   289|// downloadPricingData 从远程下载价格数据
   290|func (s *PricingService) downloadPricingData() error {
   291|	remoteURL, err := s.validatePricingURL(s.cfg.Pricing.RemoteURL)
   292|	if err != nil {
   293|		return err
   294|	}
   295|	logger.LegacyPrintf("service.pricing", "[Pricing] Downloading from %s", remoteURL)
   296|
   297|	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   298|	defer cancel()
   299|
   300|	// 获取远程哈希（用于同步锚点，不作为完整性校验）
   301|	var remoteHash string
   302|	if strings.TrimSpace(s.cfg.Pricing.HashURL) != "" {
   303|		remoteHash, err = s.fetchRemoteHash()
   304|		if err != nil {
   305|			logger.LegacyPrintf("service.pricing", "[Pricing] Failed to fetch remote hash (continuing): %v", err)
   306|		}
   307|	}
   308|
   309|	body, err := s.remoteClient.FetchPricingJSON(ctx, remoteURL)
   310|	if err != nil {
   311|		return fmt.Errorf("download failed: %w", err)
   312|	}
   313|
   314|	// 哈希校验：不匹配时仅告警，不阻止更新
   315|	// 远程哈希文件可能与数据文件不同步（如维护者更新了数据但未更新哈希文件）
   316|	dataHash := sha256.Sum256(body)
   317|	dataHashStr := hex.EncodeToString(dataHash[:])
   318|	if remoteHash != "" && !strings.EqualFold(remoteHash, dataHashStr) {
   319|		logger.LegacyPrintf("service.pricing", "[Pricing] Hash mismatch warning: remote=%s data=%s (hash file may be out of sync)",
   320|			remoteHash[:min(8, len(remoteHash))], dataHashStr[:8])
   321|	}
   322|
   323|	// 解析JSON数据（使用灵活的解析方式）
   324|	data, err := s.parsePricingData(body)
   325|	if err != nil {
   326|		return fmt.Errorf("parse pricing data: %w", err)
   327|	}
   328|
   329|	// 保存到本地文件
   330|	pricingFile := s.getPricingFilePath()
   331|	if err := os.WriteFile(pricingFile, body, 0644); err != nil {
   332|		logger.LegacyPrintf("service.pricing", "[Pricing] Failed to save file: %v", err)
   333|	}
   334|
   335|	// 使用远程哈希作为同步锚点，防止重复下载
   336|	// 当远程哈希不可用时，回退到数据本身的哈希
   337|	syncHash := dataHashStr
   338|	if remoteHash != "" {
   339|		syncHash = remoteHash
   340|	}
   341|	hashFile := s.getHashFilePath()
   342|	if err := os.WriteFile(hashFile, []byte(syncHash+"\n"), 0644); err != nil {
   343|		logger.LegacyPrintf("service.pricing", "[Pricing] Failed to save hash: %v", err)
   344|	}
   345|
   346|	// 更新内存数据
   347|	s.mu.Lock()
   348|	s.pricingData = data
   349|	s.lastUpdated = time.Now()
   350|	s.localHash = syncHash
   351|	s.mu.Unlock()
   352|
   353|	logger.LegacyPrintf("service.pricing", "[Pricing] Downloaded %d models successfully", len(data))
   354|	return nil
   355|}
   356|
   357|// parsePricingData 解析价格数据（处理各种格式）
   358|func (s *PricingService) parsePricingData(body []byte) (map[string]*LiteLLMModelPricing, error) {
   359|	// 首先解析为 map[string]json.RawMessage
   360|	var rawData map[string]json.RawMessage
   361|	if err := json.Unmarshal(body, &rawData); err != nil {
   362|		return nil, fmt.Errorf("parse raw JSON: %w", err)
   363|	}
   364|
   365|	result := make(map[string]*LiteLLMModelPricing)
   366|	skipped := 0
   367|
   368|	for modelName, rawEntry := range rawData {
   369|		// 跳过 sample_spec 等文档条目
   370|		if modelName == "sample_spec" {
   371|			continue
   372|		}
   373|
   374|		// 尝试解析每个条目
   375|		var entry LiteLLMRawEntry
   376|		if err := json.Unmarshal(rawEntry, &entry); err != nil {
   377|			skipped++
   378|			continue
   379|		}
   380|
   381|		// 只保留有有效价格的条目
   382|		if entry.InputCostPerToken == nil && entry.OutputCostPerToken == nil {
   383|			continue
   384|		}
   385|
		pricing := &LiteLLMModelPricing{
			LiteLLMProvider:           entry.LiteLLMProvider,
			Mode:                      entry.Mode,
			SupportsPromptCaching:     entry.SupportsPromptCaching,
			SupportedModalities:       append([]string(nil), entry.SupportedModalities...),
			SupportedOutputModalities: append([]string(nil), entry.SupportedOutputModalities...),
			SupportsPDFInput:          entry.SupportsPDFInput,
			SupportsServiceTier:       entry.SupportsServiceTier,
		}

   393|		if entry.InputCostPerToken != nil {
   394|			pricing.InputCostPerToken = *entry.InputCostPerToken
   395|		}
   396|		if entry.InputCostPerTokenPriority != nil {
   397|			pricing.InputCostPerTokenPriority = *entry.InputCostPerTokenPriority
   398|		}
   399|		if entry.OutputCostPerToken != nil {
   400|			pricing.OutputCostPerToken = *entry.OutputCostPerToken
   401|		}
   402|		if entry.OutputCostPerTokenPriority != nil {
   403|			pricing.OutputCostPerTokenPriority = *entry.OutputCostPerTokenPriority
   404|		}
   405|		if entry.CacheCreationInputTokenCost != nil {
   406|			pricing.CacheCreationInputTokenCost = *entry.CacheCreationInputTokenCost
   407|		}
   408|		if entry.CacheCreationInputTokenCostAbove1hr != nil {
   409|			pricing.CacheCreationInputTokenCostAbove1hr = *entry.CacheCreationInputTokenCostAbove1hr
   410|		}
   411|		if entry.CacheReadInputTokenCost != nil {
   412|			pricing.CacheReadInputTokenCost = *entry.CacheReadInputTokenCost
   413|		}
   414|		if entry.CacheReadInputTokenCostPriority != nil {
   415|			pricing.CacheReadInputTokenCostPriority = *entry.CacheReadInputTokenCostPriority
   416|		}
   417|		if entry.OutputCostPerImage != nil {
   418|			pricing.OutputCostPerImage = *entry.OutputCostPerImage
   419|		}
   420|		if entry.OutputCostPerImageToken != nil {
   421|			pricing.OutputCostPerImageToken = *entry.OutputCostPerImageToken
   422|		}
   423|		if entry.MaxInputTokens != nil {
   424|			pricing.MaxInputTokens = *entry.MaxInputTokens
   425|		}
   426|		if entry.MaxOutputTokens != nil {
   427|			pricing.MaxOutputTokens = *entry.MaxOutputTokens
   428|		}
   429|		if entry.MaxTokens != nil {
   430|			pricing.MaxTokens = *entry.MaxTokens
   431|		}
   432|
   433|		result[modelName] = pricing
   434|	}
   435|
   436|	if skipped > 0 {
   437|		logger.LegacyPrintf("service.pricing", "[Pricing] Skipped %d invalid entries", skipped)
   438|	}
   439|
   440|	if len(result) == 0 {
   441|		return nil, fmt.Errorf("no valid pricing entries found")
   442|	}
   443|
   444|	return result, nil
   445|}
   446|
   447|// loadPricingData 从本地文件加载价格数据
   448|func (s *PricingService) loadPricingData(filePath string) error {
   449|	data, err := os.ReadFile(filePath)
   450|	if err != nil {
   451|		return fmt.Errorf("read file failed: %w", err)
   452|	}
   453|
   454|	// 使用灵活的解析方式
   455|	pricingData, err := s.parsePricingData(data)
   456|	if err != nil {
   457|		return fmt.Errorf("parse pricing data: %w", err)
   458|	}
   459|
   460|	// 计算哈希
   461|	hash := sha256.Sum256(data)
   462|	hashStr := hex.EncodeToString(hash[:])
   463|
   464|	s.mu.Lock()
   465|	s.pricingData = pricingData
   466|	s.localHash = hashStr
   467|
   468|	info, _ := os.Stat(filePath)
   469|	if info != nil {
   470|		s.lastUpdated = info.ModTime()
   471|	} else {
   472|		s.lastUpdated = time.Now()
   473|	}
   474|	s.mu.Unlock()
   475|
   476|	logger.LegacyPrintf("service.pricing", "[Pricing] Loaded %d models from %s", len(pricingData), filePath)
   477|	return nil
   478|}
   479|
   480|// useFallbackPricing 使用回退价格文件
   481|func (s *PricingService) useFallbackPricing() error {
   482|	fallbackFile := s.cfg.Pricing.FallbackFile
   483|
   484|	if _, err := os.Stat(fallbackFile); os.IsNotExist(err) {
   485|		return fmt.Errorf("fallback file not found: %s", fallbackFile)
   486|	}
   487|
   488|	logger.LegacyPrintf("service.pricing", "[Pricing] Using fallback file: %s", fallbackFile)
   489|
   490|	// 复制到数据目录
   491|	data, err := os.ReadFile(fallbackFile)
   492|	if err != nil {
   493|		return fmt.Errorf("read fallback failed: %w", err)
   494|	}
   495|
   496|	pricingFile := s.getPricingFilePath()
   497|	if err := os.WriteFile(pricingFile, data, 0644); err != nil {
   498|		logger.LegacyPrintf("service.pricing", "[Pricing] Failed to copy fallback: %v", err)
   499|	}
   500|
   501|	return s.loadPricingData(fallbackFile)
   502|}
   503|
   504|// fetchRemoteHash 从远程获取哈希值
   505|func (s *PricingService) fetchRemoteHash() (string, error) {
   506|	hashURL, err := s.validatePricingURL(s.cfg.Pricing.HashURL)
   507|	if err != nil {
   508|		return "", err
   509|	}
   510|
   511|	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
   512|	defer cancel()
   513|
   514|	hash, err := s.remoteClient.FetchHashText(ctx, hashURL)
   515|	if err != nil {
   516|		return "", err
   517|	}
   518|	return strings.TrimSpace(hash), nil
   519|}
   520|
   521|func (s *PricingService) validatePricingURL(raw string) (string, error) {
   522|	if s.cfg != nil && !s.cfg.Security.URLAllowlist.Enabled {
   523|		normalized, err := urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
   524|		if err != nil {
   525|			return "", fmt.Errorf("invalid pricing url: %w", err)
   526|		}
   527|		return normalized, nil
   528|	}
   529|	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
   530|		AllowedHosts:     s.cfg.Security.URLAllowlist.PricingHosts,
   531|		RequireAllowlist: true,
   532|		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
   533|	})
   534|	if err != nil {
   535|		return "", fmt.Errorf("invalid pricing url: %w", err)
   536|	}
   537|	return normalized, nil
   538|}
   539|
   540|// GetModelPricing 获取模型价格（带模糊匹配）
   541|func (s *PricingService) GetModelPricing(modelName string) *LiteLLMModelPricing {
   542|	s.mu.RLock()
   543|	defer s.mu.RUnlock()
   544|
   545|	return s.getModelPricingLocked(modelName)
   546|}
   547|
   548|func (s *PricingService) getModelPricingLocked(modelName string) *LiteLLMModelPricing {
   549|	if modelName == "" {
   550|		return nil
   551|	}
   552|
   553|	// 标准化模型名称（同时兼容 "models/xxx"、VertexAI 资源名等前缀）
   554|	modelLower := strings.ToLower(strings.TrimSpace(modelName))
   555|	lookupCandidates := s.buildModelLookupCandidates(modelLower)
   556|
   557|	// 1. 精确匹配
   558|	for _, candidate := range lookupCandidates {
   559|		if candidate == "" {
   560|			continue
   561|		}
   562|		if pricing, ok := s.pricingData[candidate]; ok {
   563|			return pricing
   564|		}
   565|	}
   566|
   567|	// 2. 处理常见的模型名称变体
   568|	// claude-opus-4-5-20251101 -> claude-opus-4.5-20251101
   569|	for _, candidate := range lookupCandidates {
   570|		normalized := strings.ReplaceAll(candidate, "-4-5-", "-4.5-")
   571|		if pricing, ok := s.pricingData[normalized]; ok {
   572|			return pricing
   573|		}
   574|	}
   575|
   576|	// 3. 尝试模糊匹配（去掉版本号后缀）
   577|	// claude-opus-4-5-20251101 -> claude-opus-4.5
   578|	baseName := s.extractBaseName(lookupCandidates[0])
   579|	for key, pricing := range s.pricingData {
   580|		keyBase := s.extractBaseName(strings.ToLower(key))
   581|		if keyBase == baseName {
   582|			return pricing
   583|		}
   584|	}
   585|
   586|	// 4. 基于模型系列匹配（Claude）
   587|	if pricing := s.matchByModelFamily(lookupCandidates[0]); pricing != nil {
   588|		return pricing
   589|	}
   590|
   591|	// 5. OpenAI 模型回退策略
   592|	if strings.HasPrefix(lookupCandidates[0], "gpt-") {
   593|		return s.matchOpenAIModel(lookupCandidates[0])
   594|	}
   595|
   596|	return nil
   597|}
   598|
   599|func GetDefaultModelMetadata(modelName string) *LiteLLMModelPricing {
   600|	meta := modelmetadata.GetDefaultModelMetadata(modelName)
   601|	if meta == nil {
   602|		return nil
   603|	}
   604|	return &LiteLLMModelPricing{
   605|		InputCostPerToken:           meta.InputCostPerToken,
   606|		OutputCostPerToken:          meta.OutputCostPerToken,
   607|		CacheReadInputTokenCost:     meta.CacheReadInputTokenCost,
   608|		CacheCreationInputTokenCost: meta.CacheCreationInputTokenCost,
   609|		MaxInputTokens:              meta.MaxInputTokens,
   610|		MaxOutputTokens:             meta.MaxOutputTokens,
   611|		MaxTokens:                   meta.MaxTokens,
   612|	}
   613|}
   614|
   615|func (s *PricingService) buildModelLookupCandidates(modelLower string) []string {
   616|	// Prefer canonical model name first (this also improves billing compatibility with "models/xxx").
   617|	candidates := []string{
   618|		normalizeModelNameForPricing(modelLower),
   619|		modelLower,
   620|	}
   621|	candidates = append(candidates,
   622|		strings.TrimPrefix(modelLower, "models/"),
   623|		lastSegment(modelLower),
   624|		lastSegment(strings.TrimPrefix(modelLower, "models/")),
   625|	)
   626|
   627|	seen := make(map[string]struct{}, len(candidates))
   628|	out := make([]string, 0, len(candidates))
   629|	for _, c := range candidates {
   630|		c = strings.TrimSpace(c)
   631|		if c == "" {
   632|			continue
   633|		}
   634|		if _, ok := seen[c]; ok {
   635|			continue
   636|		}
   637|		seen[c] = struct{}{}
   638|		out = append(out, c)
   639|	}
   640|	if len(out) == 0 {
   641|		return []string{modelLower}
   642|	}
   643|	return out
   644|}
   645|
   646|func normalizeModelNameForPricing(model string) string {
   647|	// Common Gemini/VertexAI forms:
   648|	// - models/gemini-2.0-flash-exp
   649|	// - publishers/google/models/gemini-2.5-pro
   650|	// - projects/.../locations/.../publishers/google/models/gemini-2.5-pro
   651|	model = strings.TrimSpace(model)
   652|	model = strings.TrimLeft(model, "/")
   653|	model = strings.TrimPrefix(model, "models/")
   654|	model = strings.TrimPrefix(model, "publishers/google/models/")
   655|
   656|	if idx := strings.LastIndex(model, "/publishers/google/models/"); idx != -1 {
   657|		model = model[idx+len("/publishers/google/models/"):]
   658|	}
   659|	if idx := strings.LastIndex(model, "/models/"); idx != -1 {
   660|		model = model[idx+len("/models/"):]
   661|	}
   662|
   663|	model = strings.TrimLeft(model, "/")
   664|	if canonical := canonicalizeOpenAIModelAliasSpelling(model); canonical != "" {
   665|		return canonical
   666|	}
   667|	return model
   668|}
   669|
   670|func lastSegment(model string) string {
   671|	if idx := strings.LastIndex(model, "/"); idx != -1 {
   672|		return model[idx+1:]
   673|	}
   674|	return model
   675|}
   676|
   677|// extractBaseName 提取基础模型名称（去掉日期版本号）
   678|func (s *PricingService) extractBaseName(model string) string {
   679|	// 移除日期后缀 (如 -20251101, -20241022)
   680|	parts := strings.Split(model, "-")
   681|	result := make([]string, 0, len(parts))
   682|	for _, part := range parts {
   683|		// 跳过看起来像日期的部分（8位数字）
   684|		if len(part) == 8 && isNumeric(part) {
   685|			continue
   686|		}
   687|		// 跳过版本号（如 v1:0）
   688|		if strings.Contains(part, ":") {
   689|			continue
   690|		}
   691|		result = append(result, part)
   692|	}
   693|	return strings.Join(result, "-")
   694|}
   695|
   696|// matchByModelFamily 基于模型系列匹配
   697|func (s *PricingService) matchByModelFamily(model string) *LiteLLMModelPricing {
   698|	// modelFamily 定义一个模型系列的匹配和定价查找规则。
   699|	type modelFamily struct {
   700|		name    string   // 系列名称
   701|		match   []string // 用于将模型归类到此系列的模式（strings.Contains 匹配）
   702|		pricing []string // 用于在定价数据中查找价格的模式（nil 则复用 match；可包含低版本 fallback）
   703|	}
   704|
   705|	// 按特异性降序排列：高版本号在前，避免 "claude-opus-4"（opus-4 系列）
   706|	// 因子串关系误匹配 "claude-opus-4-7"（opus-4.7 系列）。
   707|	// 注意：原 map 实现存在 Go map 迭代随机性导致的同类 bug，此处改为有序切片修复。
   708|	families := []modelFamily{
   709|		{name: "opus-4.7", match: []string{"claude-opus-4-7", "claude-opus-4.7"}, pricing: []string{"claude-opus-4-7", "claude-opus-4.7", "claude-opus-4-6"}},
   710|		{name: "opus-4.6", match: []string{"claude-opus-4-6", "claude-opus-4.6"}},
   711|		{name: "opus-4.5", match: []string{"claude-opus-4-5", "claude-opus-4.5"}},
   712|		{name: "opus-4", match: []string{"claude-opus-4", "claude-3-opus"}},
   713|		{name: "sonnet-4.5", match: []string{"claude-sonnet-4-5", "claude-sonnet-4.5"}},
   714|		{name: "sonnet-4", match: []string{"claude-sonnet-4", "claude-3-5-sonnet"}},
   715|		{name: "sonnet-3.5", match: []string{"claude-3-5-sonnet", "claude-3.5-sonnet"}},
   716|		{name: "sonnet-3", match: []string{"claude-3-sonnet"}},
   717|		{name: "haiku-3.5", match: []string{"claude-3-5-haiku", "claude-3.5-haiku"}},
   718|		{name: "haiku-3", match: []string{"claude-3-haiku"}},
   719|	}
   720|
   721|	// Phase 1: 按有序切片归类（最具体的系列优先匹配）
   722|	var matched *modelFamily
   723|	for i := range families {
   724|		for _, pattern := range families[i].match {
   725|			if strings.Contains(model, pattern) || strings.Contains(model, strings.ReplaceAll(pattern, "-", "")) {
   726|				matched = &families[i]
   727|				break
   728|			}
   729|		}
   730|		if matched != nil {
   731|			break
   732|		}
   733|	}
   734|
   735|	// Phase 2: 二次兜底——当模型 ID 不含已知模式串时，按关键字粗分
   736|	if matched == nil {
   737|		var fallbackName string
   738|		switch {
   739|		case strings.Contains(model, "opus"):
   740|			switch {
   741|			case strings.Contains(model, "4.7") || strings.Contains(model, "4-7"):
   742|				fallbackName = "opus-4.7"
   743|			case strings.Contains(model, "4.6") || strings.Contains(model, "4-6"):
   744|				fallbackName = "opus-4.6"
   745|			case strings.Contains(model, "4.5") || strings.Contains(model, "4-5"):
   746|				fallbackName = "opus-4.5"
   747|			default:
   748|				fallbackName = "opus-4"
   749|			}
   750|		case strings.Contains(model, "sonnet"):
   751|			switch {
   752|			case strings.Contains(model, "4.5") || strings.Contains(model, "4-5"):
   753|				fallbackName = "sonnet-4.5"
   754|			case strings.Contains(model, "3-5") || strings.Contains(model, "3.5"):
   755|				fallbackName = "sonnet-3.5"
   756|			default:
   757|				fallbackName = "sonnet-4"
   758|			}
   759|		case strings.Contains(model, "haiku"):
   760|			switch {
   761|			case strings.Contains(model, "3-5") || strings.Contains(model, "3.5"):
   762|				fallbackName = "haiku-3.5"
   763|			default:
   764|				fallbackName = "haiku-3"
   765|			}
   766|		}
   767|		if fallbackName != "" {
   768|			for i := range families {
   769|				if families[i].name == fallbackName {
   770|					matched = &families[i]
   771|					break
   772|				}
   773|			}
   774|		}
   775|	}
   776|
   777|	if matched == nil {
   778|		return nil
   779|	}
   780|
   781|	// Phase 3: 在定价数据中查找该系列的价格
   782|	lookups := matched.pricing
   783|	if lookups == nil {
   784|		lookups = matched.match
   785|	}
   786|	for _, pattern := range lookups {
   787|		for key, pricing := range s.pricingData {
   788|			keyLower := strings.ToLower(key)
   789|			if strings.Contains(keyLower, pattern) {
   790|				logger.LegacyPrintf("service.pricing", "[Pricing] Fuzzy matched %s -> %s", model, key)
   791|				return pricing
   792|			}
   793|		}
   794|	}
   795|
   796|	return nil
   797|}
   798|
   799|// matchOpenAIModel OpenAI 模型回退匹配策略
   800|// 回退顺序：
   801|// 1. gpt-5.3-codex-spark* -> gpt-5.1-codex（按业务要求固定计费）
   802|// 2. gpt-5.2-codex -> gpt-5.2（去掉后缀如 -codex, -mini, -max 等）
   803|// 3. gpt-5.2-20251222 -> gpt-5.2（去掉日期版本号）
   804|// 4. gpt-5.3-codex -> gpt-5.2-codex
   805|// 5. gpt-5.4* -> 业务静态兜底价
   806|// 6. 最终回退到 DefaultTestModel (gpt-5.1-codex)
   807|func (s *PricingService) matchOpenAIModel(model string) *LiteLLMModelPricing {
   808|	if strings.HasPrefix(model, "gpt-5.3-codex-spark") {
   809|		if pricing, ok := s.pricingData["gpt-5.1-codex"]; ok {
   810|			logger.LegacyPrintf("service.pricing", "[Pricing][SparkBilling] %s -> %s billing", model, "gpt-5.1-codex")
   811|			logger.With(zap.String("component", "service.pricing")).
   812|				Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.1-codex"))
   813|			return pricing
   814|		}
   815|	}
   816|
   817|	// 尝试的回退变体
   818|	variants := s.generateOpenAIModelVariants(model, openAIModelDatePattern)
   819|
   820|	for _, variant := range variants {
   821|		if pricing, ok := s.pricingData[variant]; ok {
   822|			logger.With(zap.String("component", "service.pricing")).
   823|				Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, variant))
   824|			return pricing
   825|		}
   826|	}
   827|
   828|	if strings.HasPrefix(model, "gpt-5.3-codex") {
   829|		if pricing, ok := s.pricingData["gpt-5.2-codex"]; ok {
   830|			logger.With(zap.String("component", "service.pricing")).
   831|				Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.2-codex"))
   832|			return pricing
   833|		}
   834|	}
   835|
   836|	// GPT-5.5 回退到 GPT-5.4 定价
   837|	if strings.HasPrefix(model, "gpt-5.5") {
   838|		logger.With(zap.String("component", "service.pricing")).
   839|			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.4(static)"))
   840|		return openAIGPT54FallbackPricing
   841|	}
   842|
   843|	if strings.HasPrefix(model, "gpt-5.4-mini") {
   844|		logger.With(zap.String("component", "service.pricing")).
   845|			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.4-mini(static)"))
   846|		return openAIGPT54MiniFallbackPricing
   847|	}
   848|
   849|	if strings.HasPrefix(model, "gpt-5.4-nano") {
   850|		logger.With(zap.String("component", "service.pricing")).
   851|			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.4-nano(static)"))
   852|		return openAIGPT54NanoFallbackPricing
   853|	}
   854|
   855|	if strings.HasPrefix(model, "gpt-5.4") {
   856|		logger.With(zap.String("component", "service.pricing")).
   857|			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.4(static)"))
   858|		return openAIGPT54FallbackPricing
   859|	}
   860|
   861|	if isOpenAIImageGenerationModel(model) {
   862|		for _, candidate := range []string{"gpt-image-2", "gpt-image-1.5", "gpt-image-1"} {
   863|			if pricing, ok := s.pricingData[candidate]; ok {
   864|				logger.LegacyPrintf("service.pricing", "[Pricing] OpenAI image fallback matched %s -> %s", model, candidate)
   865|				return pricing
   866|			}
   867|		}
   868|		return nil
   869|	}
   870|
   871|	// 最终回退到 DefaultTestModel
   872|	defaultModel := strings.ToLower(openai.DefaultTestModel)
   873|	if pricing, ok := s.pricingData[defaultModel]; ok {
   874|		logger.LegacyPrintf("service.pricing", "[Pricing] OpenAI fallback to default model %s -> %s", model, defaultModel)
   875|		return pricing
   876|	}
   877|
   878|	return nil
   879|}
   880|
   881|// generateOpenAIModelVariants 生成 OpenAI 模型的回退变体列表
   882|func (s *PricingService) generateOpenAIModelVariants(model string, datePattern *regexp.Regexp) []string {
   883|	seen := make(map[string]bool)
   884|	var variants []string
   885|
   886|	addVariant := func(v string) {
   887|		if v != model && !seen[v] {
   888|			seen[v] = true
   889|			variants = append(variants, v)
   890|		}
   891|	}
   892|
   893|	// 1. 去掉日期版本号: gpt-5.2-20251222 -> gpt-5.2
   894|	withoutDate := datePattern.ReplaceAllString(model, "")
   895|	if withoutDate != model {
   896|		addVariant(withoutDate)
   897|	}
   898|
   899|	// 2. 提取基础版本号: gpt-5.2-codex -> gpt-5.2
   900|	// 只匹配纯数字版本号格式 gpt-X 或 gpt-X.Y，不匹配 gpt-4o 这种带字母后缀的
   901|	if matches := openAIModelBasePattern.FindStringSubmatch(model); len(matches) > 1 {
   902|		addVariant(matches[1])
   903|	}
   904|
   905|	// 3. 同时去掉日期后再提取基础版本号
   906|	if withoutDate != model {
   907|		if matches := openAIModelBasePattern.FindStringSubmatch(withoutDate); len(matches) > 1 {
   908|			addVariant(matches[1])
   909|		}
   910|	}
   911|
   912|	return variants
   913|}
   914|
   915|// GetStatus 获取服务状态
   916|func (s *PricingService) GetStatus() map[string]any {
   917|	s.mu.RLock()
   918|	defer s.mu.RUnlock()
   919|
   920|	return map[string]any{
   921|		"model_count":  len(s.pricingData),
   922|		"last_updated": s.lastUpdated,
   923|		"local_hash":   s.localHash[:min(8, len(s.localHash))],
   924|	}
   925|}
   926|
   927|// ForceUpdate 强制更新
   928|func (s *PricingService) ForceUpdate() error {
   929|	return s.downloadPricingData()
   930|}
   931|
   932|// getPricingFilePath 获取价格文件路径
   933|func (s *PricingService) getPricingFilePath() string {
   934|	return filepath.Join(s.cfg.Pricing.DataDir, "model_pricing.json")
   935|}
   936|
   937|// getHashFilePath 获取哈希文件路径
   938|func (s *PricingService) getHashFilePath() string {
   939|	return filepath.Join(s.cfg.Pricing.DataDir, "model_pricing.sha256")
   940|}
   941|
   942|// isNumeric 检查字符串是否为纯数字
   943|func isNumeric(s string) bool {
   944|	for _, c := range s {
   945|		if c < '0' || c > '9' {
   946|			return false
   947|		}
   948|	}
   949|	return true
   950|}
   951|