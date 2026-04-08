#!/bin/bash

docker build -t ccr.ccs.tencentyun.com/afan-public/kaniko:test1 .

docker push ccr.ccs.tencentyun.com/afan-public/kaniko:test1