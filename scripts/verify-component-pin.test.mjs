import fs from 'node:fs'
import assert from 'node:assert/strict'
import test from 'node:test'
import {validatePin,validateResolution} from './verify-component-pin.mjs'
const pin=JSON.parse(fs.readFileSync(new URL('../host/COMPONENT_PIN.json',import.meta.url)))
test('accepted component pin has an immutable owner identity',()=>validatePin(pin))
for(const [field,value]of Object.entries({componentID:'foreign',module:'example.invalid/component',version:'main',sourceCommit:'mutable',moduleChecksum:'unverified'}))test(`reject altered component ${field}`,()=>assert.throws(()=>validatePin({...pin,[field]:value})))
const resolution={Path:pin.module,Version:pin.version,Sum:pin.moduleChecksum,GoModSum:pin.goModChecksum,Origin:{VCS:'git',Hash:pin.sourceCommit}}
test('reject component source substitution and checksum drift',()=>{
 validateResolution(resolution,pin)
 for(const change of [{Replace:{Dir:'../local'}},{Main:true},{Version:'v9.9.9'},{Sum:'h1:wrong'},{Origin:{VCS:'git',Hash:'0'.repeat(40)}}])assert.throws(()=>validateResolution({...resolution,...change},pin))
})
