# nocgo
yet another way to use c/asm in golang, translate asm to goasm

## TODO
- [ ] x86 arch

## dependence

### keystone
1. use my fork https://github.com/kkHAIKE/keystone/tree/fix_adr , it's fix ADR instruction at arm64.
2. use homebrew in macos:
    1. `brew edit keystone`
    2. replace `head "https://github.com/keystone-engine/keystone.git", branch: "master"` to `head "https://github.com/kkHAIKE/keystone.git", branch: "fix_adr"`
    3. `brew install --head keystone`

### golang
let us wait next release version

this commit fix `WORD $0` bug:
https://github.com/golang/go/commit/9f0f87c806b7a11b2cb3ebcd02eac57ee389c43a

## build
1. ``` export CGO_CFLAGS=`pkg-config --cflags keystone` ```
2. ``` export CGO_LDFLAGS=`pkg-config --libs keystone` ```
3. `go install`
