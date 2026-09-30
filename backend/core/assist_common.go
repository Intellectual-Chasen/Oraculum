package core

// AssistProvider は、分析者の証拠を送る LLM の提供者である。
type AssistProvider string

// AssistProvider の値。
const (
	// AssistProviderClaude は、分析者本人がログインした Claude の CLI である。
	AssistProviderClaude AssistProvider = "claude"
)

// AssistProviders は AssistProvider の値を定義の順で返す。
func AssistProviders() []AssistProvider {
	return []AssistProvider{AssistProviderClaude}
}

// IsKnown は値が定義の中にあることを返す。
func (p AssistProvider) IsKnown() bool {
	for _, known := range AssistProviders() {
		if p == known {
			return true
		}
	}
	return false
}

// assistConversationIdLength は会話の識別子の字句の長さである。乱数 128 bit を小文字の 16 進で
// 書いた長さである。
const assistConversationIdLength = 32

// ValidateAssistConversationId は、会話の識別子が乱数 128 bit を小文字の 16 進で書いた字句で
// あることを確かめる。**識別子の字句を error に載せない。**
func ValidateAssistConversationId(item, value string) error {
	if len(value) != assistConversationIdLength {
		return itemError(item, ErrInvalid)
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return itemError(item, ErrInvalid)
		}
	}
	return nil
}

// assistTurnIdMaxLength は発言の識別子の字句の長さの上限である。
const assistTurnIdMaxLength = 64

// ValidateAssistTurnId は、発言の識別子が英数字と `-` と `_` の 1 文字以上 64 文字以下で
// あることを確かめる。**識別子の字句を error に載せない。**
func ValidateAssistTurnId(item, value string) error {
	if value == "" {
		return itemError(item, ErrMissingRequiredItem)
	}
	if len(value) > assistTurnIdMaxLength {
		return itemError(item, ErrInvalid)
	}
	for _, r := range value {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit && r != '-' && r != '_' {
			return itemError(item, ErrInvalid)
		}
	}
	return nil
}

// AssistMatchCondition は、受け渡しの本文を組んだ関連付けの条件 1 件と、その幅である。
type AssistMatchCondition struct {
	ConditionKey ConditionKey `json:"conditionKey"`
	Tolerance    int64        `json:"tolerance"`
}

// validateAssistMatchConditions は関連付けの条件の選択の要素を確かめる。
func validateAssistMatchConditions(item string, conditions []AssistMatchCondition) error {
	for _, condition := range conditions {
		if err := requireKnownEnum(item+".conditionKey", condition.ConditionKey); err != nil {
			return err
		}
		if condition.Tolerance < 0 {
			return itemError(item+".tolerance", ErrNegativeCount)
		}
	}
	return nil
}
