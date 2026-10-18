param(
  [Parameter(Mandatory = $true)][string]$Anc,
  [string]$Addr = "127.0.0.1:6379",
  [string]$Prefix = "ancdemo"
)

$ErrorActionPreference = "Stop"

$items = @(
  @{ id = "demo-prompt-baseline"; title = "Prompt 归因基线：100 条样本标注与复核"; status = "running"; by = "eng-owner"; to = "bpobot"; domain = "agent-bpo"; project = "prompt-attribution" },
  @{ id = "demo-reception-gray"; title = "接待工作台灰度：抽检 200 通会话并标注口径"; status = "running"; by = "cs-owner"; to = "receptionbot"; domain = "cs-ops"; project = "reception-workbench" },
  @{ id = "demo-shopinit-auth"; title = "定期校验店铺初始化凭据，补跑 2 个失败任务"; status = "running"; by = "delivery-owner"; to = "shopinitbot"; domain = "delivery-ops"; project = "shop-init" }
)

foreach ($item in $items) {
  & $Anc hot put $item.id `
    -status $item.status `
    -by $item.by `
    -to $item.to `
    -domain $item.domain `
    -project $item.project `
    -title $item.title `
    -addr $Addr `
    -prefix $Prefix
  if ($LASTEXITCODE -ne 0) {
    throw "anc hot put 失败：$($item.id)"
  }
}
