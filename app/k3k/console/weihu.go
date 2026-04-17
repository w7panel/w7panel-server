package console

import (
	"github.com/spf13/cobra"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type Weihu struct {
	console2.Abstract
}

func (c Weihu) GetName() string {
	return "weihu"
}

func (c Weihu) Configure(cmd *cobra.Command) {

}

func (c Weihu) GetDescription() string {
	return "维护模式job"
}

func (c Weihu) Handle(cmd *cobra.Command, args []string) {

}

/*
*
k3k 集群维护模式
*/
func (c Weihu) HandleK3k(saName, namespace string) {
	// k3kUser := k3kUser.NewK3kUser(saName, namespace)
}
