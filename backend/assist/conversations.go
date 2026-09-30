package assist

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ErrTurnInProgress は、同じ会話で前の発言への応答が終わっていないことを表す。
var ErrTurnInProgress = errors.New("assist: the previous message is still being answered")

// ErrTurnRepeated は、同じ発言の識別子を 2 回送ったことを表す。
var ErrTurnRepeated = errors.New("assist: the message identifier was already sent")

// ErrConversationStopped は、会話の提供者を止めたことを表す。分析者は新しい会話を始める。
var ErrConversationStopped = errors.New("assist: the provider of this conversation was stopped; open a new conversation")

// conversation は中継が持つ会話 1 件である。
type conversation struct {
	id       string
	provider core.AssistProvider
	// client は会話を開いたセッションで server を呼ぶ。別の browser の認証と混ぜない。
	client ServerClient
	// secret は会話の MCP の bearer secret である。
	secret  string
	workDir string
	session ProviderSession
	mu      sync.Mutex
	events  []core.AssistEvent
	// turns は受け付けた発言の識別子である。
	turns map[string]bool
	// current は応答の途中の発言である。応答の途中でなければ nil である。
	current *activeTurn
	// stopped は提供者を止めたことである。止めた会話は以後の発言を受け付けない。
	stopped bool
	// changed は event を足すたびに閉じて作り直す。待つ側は閉じたことで新しい event を知る。
	changed chan struct{}
}

// activeTurn は応答の途中の発言と、その発言の関連付けの条件である。
type activeTurn struct {
	id         string
	conditions []core.AssistMatchCondition
}

// conversationSummary は中継が持つ会話の一覧の要素である。
type conversationSummary struct {
	Id         string              `json:"id"`
	Provider   core.AssistProvider `json:"provider"`
	EventCount int                 `json:"eventCount"`
	// Answering は応答の途中の発言があることである。
	Answering bool `json:"answering"`
}

// conversations は中継が持つ会話の一覧である。
type conversations struct {
	mu    sync.Mutex
	order []*conversation
	byId  map[string]*conversation
}

func newConversations() *conversations {
	return &conversations{byId: map[string]*conversation{}}
}

// add は会話を足す。
func (c *conversations) add(item *conversation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order = append(c.order, item)
	c.byId[item.id] = item
}

// find は識別子で会話を探す。
func (c *conversations) find(id string) (*conversation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, found := c.byId[id]
	return item, found
}

// bySecret は bearer secret で会話を探す。secret の比較は時間を一定にする。
func (c *conversations) bySecret(secret string) (*conversation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, item := range c.order {
		if subtle.ConstantTimeCompare([]byte(item.secret), []byte(secret)) == 1 {
			return item, true
		}
	}
	return nil, false
}

// summaries は client と同じセッションの会話を、発行した順に返す。
func (c *conversations) summaries(client ServerClient) []conversationSummary {
	c.mu.Lock()
	items := slices.Clone(c.order)
	c.mu.Unlock()
	summaries := make([]conversationSummary, 0, len(items))
	for _, item := range items {
		if !item.client.sameSession(client) {
			continue
		}
		item.mu.Lock()
		summaries = append(summaries, conversationSummary{
			Id: item.id, Provider: item.provider, EventCount: len(item.events), Answering: item.current != nil,
		})
		item.mu.Unlock()
	}
	return summaries
}

// all は会話を発行した順に返す。
func (c *conversations) all() []*conversation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.order)
}

func newConversation(id string, provider core.AssistProvider, secret, workDir string, client ServerClient) *conversation {
	return &conversation{
		id: id, provider: provider, client: client, secret: secret, workDir: workDir, turns: map[string]bool{},
		changed: make(chan struct{}),
	}
}

// begin は発言を受け付け、応答の途中の発言にする。
func (c *conversation) begin(turnId string, conditions []core.AssistMatchCondition) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return ErrConversationStopped
	}
	if c.turns[turnId] {
		return ErrTurnRepeated
	}
	if c.current != nil {
		return ErrTurnInProgress
	}
	c.turns[turnId] = true
	c.current = &activeTurn{id: turnId, conditions: slices.Clone(conditions)}
	return nil
}

// stop は会話の提供者を止め、以後の発言を退ける会話にする。2 回目以降の呼び出しは提供者の
// Close を呼ばない。
func (c *conversation) stop() error {
	c.mu.Lock()
	alreadyStopped := c.stopped
	c.stopped = true
	session := c.session
	c.mu.Unlock()
	if alreadyStopped || session == nil {
		return nil
	}
	// lock を持たずに止める。止めた提供者の出力の終わりで Send が返り、会話に event を足す。
	return session.Close()
}

// abandon は、応答を始める前に失敗した発言を応答の途中から外す。発言の識別子は受け付けたまま残す。
func (c *conversation) abandon(turnId string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil && c.current.id == turnId {
		c.current = nil
	}
}

// activeTurn は応答の途中の発言を返す。
func (c *conversation) activeTurn() (activeTurn, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return activeTurn{}, false
	}
	return *c.current, true
}

// append は発言の event を足し、待つ側へ知らせる。発言の終わりの event は応答の途中の発言を外す。
func (c *conversation) append(turnId string, event core.AssistEvent) (core.AssistEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	event.Sequence = int64(len(c.events)) + 1
	event.TurnId = turnId
	if err := event.Validate(); err != nil {
		return core.AssistEvent{}, fmt.Errorf("recording the conversation event: %w", err)
	}
	c.events = append(c.events, event)
	if (event.Kind == core.AssistEventKindTurnEnd || event.Kind == core.AssistEventKindProviderError) &&
		c.current != nil && c.current.id == turnId {
		c.current = nil
	}
	close(c.changed)
	c.changed = make(chan struct{})
	return event, nil
}

// eventsAfter は通番が after より大きい event と、次の event を待つ channel を返す。
func (c *conversation) eventsAfter(after int64) ([]core.AssistEvent, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if after < 0 {
		after = 0
	}
	if after >= int64(len(c.events)) {
		return nil, c.changed
	}
	return slices.Clone(c.events[after:]), c.changed
}

// newSecret は乱数 256 bit の bearer secret を作る。
func newSecret() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("creating the MCP secret: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
