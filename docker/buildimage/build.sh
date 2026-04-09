#!/bin/bash

docker build -t ccr.ccs.tencentyun.com/afan-public/kaniko:test2 .

docker push ccr.ccs.tencentyun.com/afan-public/kaniko:test2