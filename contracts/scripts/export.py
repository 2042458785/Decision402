"""Run after forge build; only the reviewed wallet artifact is embedded in Go."""
import json
from pathlib import Path
root = Path(__file__).resolve().parents[2]
a = json.loads((root / 'contracts/out/DecisionWallet.sol/DecisionWallet.json').read_text())
out = {"abi": a['abi'], "bytecode": a['bytecode']['object'], "runtime": a['deployedBytecode']['object']}
(root / 'internal/agent/contracts/DecisionWallet.json').write_text(json.dumps(out, separators=(',', ':')) + '\n')
