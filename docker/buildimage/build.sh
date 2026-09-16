#!/bin/bash

docker build -t ccr.ccs.tencentyun.com/afan-public/kaniko:w7console-new5-26 .

docker push ccr.ccs.tencentyun.com/afan-public/kaniko:w7console-new5-26
