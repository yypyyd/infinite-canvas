package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/basketikun/infinite-canvas/model"
)

var productionTemplateVariablePattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

type builtinProductionTemplateDefinition struct {
	ID           string
	Name         string
	Description  string
	TemplateType model.ProductionTemplateType
	Platform     string
	Prompt       string
	SpecJSON     string
}

var builtinProductionTemplates = []builtinProductionTemplateDefinition{
	{ID: "product-main", Name: "商品主图", Description: "纯白背景的标准电商商品主图", TemplateType: model.ProductionTemplateTypeMain, Platform: "original", Prompt: "以参考图中的商品为唯一主体，保持商品外观、结构、颜色、材质和品牌细节准确一致。生成纯白背景电商主图，商品居中完整展示，占画面约 85%，柔和棚拍光，边缘清晰，真实自然阴影，无道具、无文字、无水印，专业商业摄影质感。", SpecJSON: `{"format":"png","quality":90,"defaultQuantity":1,"requireReference":true,"variables":[],"filenamePattern":"{spu}_{sku}_{template}_{variant}_{item}"}`},
	{ID: "lifestyle", Name: "生活场景", Description: "面向目标消费者的商品使用场景", TemplateType: model.ProductionTemplateTypeScene, Platform: "original", Prompt: "以参考图中的商品为核心，严格保持商品外观、颜色、比例和品牌细节。将商品置于符合目标消费者生活方式的真实使用场景中，主体醒目，环境克制，光线自然高级，画面有呼吸感与购买欲，商业摄影，细节清晰，不添加无关文字或水印。", SpecJSON: `{"format":"png","quality":90,"defaultQuantity":1,"requireReference":true,"variables":["product.targetAudience"],"filenamePattern":"{spu}_{sku}_{template}_{variant}_{item}"}`},
	{ID: "selling-points", Name: "卖点详情", Description: "突出功能和材质的详情页视觉", TemplateType: model.ProductionTemplateTypeDetail, Platform: "original", Prompt: "根据参考商品生成电商详情页视觉，准确保留商品结构、材质、颜色和品牌细节。通过局部特写与简洁构图突出核心功能和材质质感，画面层级清楚，预留干净的标题与卖点文案区域，但不要生成任何文字，专业棚拍光，高级电商详情页风格。", SpecJSON: `{"format":"png","quality":90,"defaultQuantity":1,"requireReference":true,"requireSellingPoints":true,"variables":["product.sellingPoints"],"filenamePattern":"{spu}_{sku}_{template}_{variant}_{item}"}`},
	{ID: "promotion", Name: "促销视觉", Description: "预留活动信息区域的转化视觉", TemplateType: model.ProductionTemplateTypePromotion, Platform: "original", Prompt: "以参考商品为视觉中心，保持商品本身准确一致，生成具有强转化氛围的电商促销视觉。使用有张力的构图、清晰的前后层次和适度的活动装饰，预留价格、折扣和活动标题区域，但不要生成任何文字，商品清晰醒目，商业广告质感。", SpecJSON: `{"format":"png","quality":90,"defaultQuantity":1,"requireReference":true,"variables":[],"filenamePattern":"{spu}_{sku}_{template}_{variant}_{item}"}`},
	{ID: "apparel-model", Name: "服饰模特", Description: "保持服饰细节的模特穿搭图", TemplateType: model.ProductionTemplateTypeCustom, Platform: "original", Prompt: "让专业模特自然穿着参考图中的服饰，严格保持服饰版型、颜色、图案、面料纹理和所有设计细节一致。姿态自然，搭配克制，背景干净，柔和时尚棚拍光，完整展示穿着效果，真实服装摄影，不添加文字或水印。", SpecJSON: `{"format":"png","quality":90,"defaultQuantity":1,"requireReference":true,"variables":[],"filenamePattern":"{spu}_{sku}_{template}_{variant}_{item}"}`},
	{ID: "sku-series", Name: "SKU 系列", Description: "统一构图和光线的 SKU 系列视觉", TemplateType: model.ProductionTemplateTypeSKUSeries, Platform: "original", Prompt: "基于参考图生成同一商品系列的标准化电商视觉。严格保持各 SKU 的颜色、结构、材质和区别点，统一拍摄角度、光线、背景、商品占比和阴影风格，构图规整，适合店铺列表与 SKU 选择展示，不添加文字或水印。", SpecJSON: `{"format":"png","quality":90,"defaultQuantity":1,"requireReference":true,"variables":["sku.name","sku.code"],"filenamePattern":"{spu}_{sku}_{template}_{variant}_{item}"}`},
}

func findBuiltinProductionTemplate(id string) (builtinProductionTemplateDefinition, bool) {
	for _, definition := range builtinProductionTemplates {
		if definition.ID == id { return definition, true }
	}
	return builtinProductionTemplateDefinition{}, false
}

var productionTemplateVariables = map[string]bool{
	"product.name": true, "product.category": true, "product.description": true,
	"product.sellingPoints": true, "product.targetAudience": true, "sku.name": true,
	"sku.code": true, "brand.name": true, "brand.tone": true, "brand.guidelines": true,
	"brand.prohibitedTerms": true,
}

type productionTemplateSpec struct {
	Width int `json:"width"`
	Height int `json:"height"`
	Format string `json:"format"`
	Quality int `json:"quality"`
	DefaultQuantity int `json:"defaultQuantity"`
	RequireReference bool `json:"requireReference"`
	RequireSellingPoints bool `json:"requireSellingPoints"`
	RequireBrand bool `json:"requireBrand"`
	AllowSPUWithoutSKU bool `json:"allowSpuWithoutSku"`
	Variables []string `json:"variables"`
	FilenamePattern string `json:"filenamePattern"`
}

func normalizeProductionTemplateSpec(specJSON string) (string, productionTemplateSpec, error) {
	specJSON = strings.TrimSpace(specJSON)
	if specJSON == "" { specJSON = `{"format":"png","quality":90,"defaultQuantity":1,"variables":[],"filenamePattern":"{spu}_{sku}_{template}_{variant}_{item}"}` }
	var spec productionTemplateSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil { return "", spec, safeMessageError{message: "模板规格 JSON 无效"} }
	if spec.DefaultQuantity == 0 { spec.DefaultQuantity = 1 }
	if spec.DefaultQuantity < 1 || spec.DefaultQuantity > 10 || spec.Width < 0 || spec.Height < 0 || spec.Quality < 0 || spec.Quality > 100 { return "", spec, safeMessageError{message: "模板尺寸、质量或默认数量无效"} }
	if spec.Format == "" { spec.Format = "png" }
	if spec.Format != "png" && spec.Format != "jpeg" && spec.Format != "webp" { return "", spec, safeMessageError{message: "模板输出格式无效"} }
	for _, variable := range spec.Variables { if !validProductionTemplateVariable(variable) { return "", spec, safeMessageError{message: "模板声明了不支持的变量：" + variable} } }
	value, _ := json.Marshal(spec)
	return string(value), spec, nil
}

func validateProductionTemplateVariables(prompt string) error {
	for _, match := range productionTemplateVariablePattern.FindAllStringSubmatch(prompt, -1) {
		if !validProductionTemplateVariable(strings.TrimSpace(match[1])) { return safeMessageError{message: "模板包含不支持的变量：" + strings.TrimSpace(match[1])} }
	}
	return nil
}

func validProductionTemplateVariable(variable string) bool {
	return productionTemplateVariables[variable] || (strings.HasPrefix(variable, "sku.attributes.") && len(strings.TrimPrefix(variable, "sku.attributes.")) <= 80)
}

func renderProductionTemplatePrompt(prompt string, product model.Product, sku *model.ProductSKU, brand *model.Brand) (string, error) {
	if err := validateProductionTemplateVariables(prompt); err != nil {
		return "", err
	}
	values := map[string]string{
		"product.name":            product.Name,
		"product.category":        product.Category,
		"product.description":     product.Description,
		"product.sellingPoints":   strings.Join(product.SellingPoints, "；"),
		"product.targetAudience":  product.TargetAudience,
		"sku.name":                "",
		"sku.code":                "",
		"brand.name":              "",
		"brand.tone":              "",
		"brand.guidelines":        "",
		"brand.prohibitedTerms":   "",
	}
	if sku != nil {
		values["sku.name"] = sku.Name
		values["sku.code"] = sku.Code
	}
	if brand != nil {
		values["brand.name"] = brand.Name
		values["brand.tone"] = brand.Tone
		values["brand.guidelines"] = brand.Guidelines
		values["brand.prohibitedTerms"] = strings.Join(brand.ProhibitedTerms, "、")
	}
	return productionTemplateVariablePattern.ReplaceAllStringFunc(prompt, func(match string) string {
		parts := productionTemplateVariablePattern.FindStringSubmatch(match)
		variable := strings.TrimSpace(parts[1])
		if strings.HasPrefix(variable, "sku.attributes.") {
			if sku == nil {
				return ""
			}
			return sku.Attributes[strings.TrimPrefix(variable, "sku.attributes.")]
		}
		return values[variable]
	}), nil
}
