#!/bin/bash

export SPEECHSDK_ROOT="$HOME/SpeechSDK"
export CGO_CFLAGS="-I$SPEECHSDK_ROOT/include/c_api"
export CGO_LDFLAGS="-L$SPEECHSDK_ROOT/lib/x64 -lMicrosoft.CognitiveServices.Speech.core"
export LD_LIBRARY_PATH="$SPEECHSDK_ROOT/lib/x64:$LD_LIBRARY_PATH"

export GOROOT="/home/washerd/sdk/go1.25.1"
export PATH="$PATH:$GOROOT/bin"

echo "🎙️  Azure Speech SDK environment configured"
echo "📍 Speech SDK: $SPEECHSDK_ROOT"
echo "🔧 CGO_CFLAGS: $CGO_CFLAGS"
echo "📚 LD_LIBRARY_PATH: $LD_LIBRARY_PATH"
echo ""

if [ $# -eq 0 ]; then
    echo "Starting new shell with Azure Speech SDK environment..."
    exec $SHELL
else
    echo "Running: $@"
    exec "$@"
fi