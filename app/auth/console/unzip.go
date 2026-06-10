package console

import (
	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/helper"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type Unzip struct {
	console2.Abstract
}

type unzipOption struct {
	zipPath    string
	targetPath string
	decodeGBk  bool
}

var unzipOp = unzipOption{}

func (c Unzip) GetName() string {
	return "unzip"
}

func (c Unzip) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&unzipOp.zipPath, "zipPath", "", "zip文件路径")
	cmd.Flags().StringVar(&unzipOp.targetPath, "targetPath", "", "目标路径")
	cmd.Flags().BoolVar(&unzipOp.decodeGBk, "decodeGBK", false, "是否解码GBK编码")
}

func (c Unzip) GetDescription() string {
	return "解压文件"
}

func (c Unzip) Handle(cmd *cobra.Command, args []string) {
	helper.Unzip(unzipOp.zipPath, unzipOp.targetPath, unzipOp.decodeGBk)
}
