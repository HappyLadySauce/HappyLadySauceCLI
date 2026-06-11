package runner

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
)


type Runner struct {
	Model *model.BaseChatModel
	Agent *adk.ChatModelAgent
}
