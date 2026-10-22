import fs from 'node:fs'
import path from 'node:path'
import {createHash} from 'node:crypto'
import {fileURLToPath} from 'node:url'
const root=fileURLToPath(new URL('../',import.meta.url))
const read=name=>fs.readFileSync(path.join(root,name),'utf8').replaceAll('\r\n','\n')
const digest=text=>createHash('sha256').update(text).digest('hex')
const pin=JSON.parse(read('contracts/PIN.json')),manifestText=read('contracts/management/v1/manifest.json')
const manifest=JSON.parse(manifestText)
if(pin.canonicalRepository!=='MalenkiySolovey/solovey-ui'||!/^[a-f0-9]{40}$/.test(pin.panelContractCommit))throw Error('Invalid canonical provenance')
if(pin.manifestSHA256!==digest(manifestText))throw Error('Pinned manifest drift')
for(const key of ['contractVersion','schemaSHA256','generatedSHA256','vectorsSHA256','bundleSHA256'])
  if(pin[key]!==manifest[key])throw Error(`Pin mismatch: ${key}`)
for(const [name,hash] of Object.entries(manifest.files))
  if(digest(read(`contracts/management/v1/${name}`))!==hash)throw Error(`Bundled file drift: ${name}`)
if(digest(read('contracts/tools/generate.mjs'))!==manifest.generator.sha256)throw Error('Generator drift')
console.log(JSON.stringify({contractVersion:pin.contractVersion,panelContractCommit:pin.panelContractCommit,bundleSHA256:pin.bundleSHA256}))
