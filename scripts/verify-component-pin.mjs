import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
import {createHash} from 'node:crypto'
import {execFileSync} from 'node:child_process'
import {fileURLToPath} from 'node:url'
import {root,validateEnvironment} from './verify-host-pin.mjs'

export function validatePin(pin){
 assert.equal(pin.schema,'solovey.component.consumer-pin/v1')
 assert.equal(pin.componentID,'observability-extra')
 assert.equal(pin.module,'github.com/MalenkiySolovey/solovey-component-observability-extra')
 assert.match(pin.version,/^v\d+\.\d+\.\d+$/)
 for(const name of ['sourceCommit','sourceTree'])assert.match(pin[name],/^[a-f0-9]{40}$/)
 for(const name of ['moduleChecksum','goModChecksum'])assert.match(pin[name],/^h1:[A-Za-z0-9+/]{43}=$/)
 assert.deepEqual(Object.keys(pin.files).sort(),['component.go','component.json','jobs/sampling.go','manifest.go'])
 for(const value of Object.values(pin.files))assert.match(value,/^[a-f0-9]{64}$/)
}
export function validateResolution(resolution,pin){
 assert.equal(resolution.Path,pin.module);assert.equal(resolution.Version,pin.version)
 assert.ok(!resolution.Error&&!resolution.Replace&&!resolution.Main,'component source substitution rejected')
 assert.equal(resolution.Sum,pin.moduleChecksum);assert.equal(resolution.GoModSum,pin.goModChecksum)
 if(resolution.Origin){assert.equal(resolution.Origin.VCS,'git');assert.equal(resolution.Origin.Hash,pin.sourceCommit)}
}
export function verify(){
 validateEnvironment(process.env)
 const pin=JSON.parse(fs.readFileSync(path.join(root,'host/COMPONENT_PIN.json')));validatePin(pin)
 const run=args=>JSON.parse(execFileSync('go',args,{cwd:root,encoding:'utf8',maxBuffer:4<<20}))
 const metadata=run(['mod','edit','-json'])
 assert.ok(!(metadata.Replace??[]).some(r=>r.Old.Path===pin.module),'component replace rejected')
 assert.equal(metadata.Require.find(r=>r.Path===pin.module)?.Version,pin.version)
 const resolution=run(['mod','download','-json',`${pin.module}@${pin.version}`]);validateResolution(resolution,pin)
 const selected=run(['list','-m','-json','-mod=readonly',pin.module]);validateResolution(selected,pin)
 assert.equal(path.resolve(selected.Dir),path.resolve(resolution.Dir))
 for(const[name,expected]of Object.entries(pin.files))assert.equal(createHash('sha256').update(fs.readFileSync(path.join(resolution.Dir,name))).digest('hex'),expected,`component lifecycle source drift: ${name}`)
 return {status:'PASS',componentID:pin.componentID,module:pin.module,version:pin.version,source:pin.sourceCommit,moduleChecksum:pin.moduleChecksum,proofLevel:'test-only shared lifecycle; no production Node'}
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url))console.log(JSON.stringify(verify()))
