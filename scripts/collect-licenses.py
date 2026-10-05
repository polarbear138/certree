"""Collect unmodified dependency license texts from the installed lockfile graph."""
import json, re, shutil, subprocess, os
from pathlib import Path
out=Path('docs/licenses');out.mkdir(parents=True,exist_ok=True)
text='\n'.join(subprocess.check_output(['go','list','-deps','-json','./apps/server'],text=True,encoding='utf-8',env={**os.environ,'GOOS':platform,'GOARCH':'amd64'}) for platform in ['windows','linux'])
decoder=json.JSONDecoder();position=0;modules={}
while position<len(text):
 while position<len(text) and text[position].isspace():position+=1
 if position>=len(text):break
 obj,end=decoder.raw_decode(text,position);position=end
 m=obj.get('Module')
 if m and m.get('Version') and m.get('Dir'):modules[m['Path']]=m
rows=[]
def copy_licenses(name,version,folder,kind,declared=''):
 files=[p for p in Path(folder).iterdir() if p.is_file() and re.match(r'^(license|licence|copying|copyright|notice)([.\-_].*)?$',p.name,re.I)]
 links=[]
 for f in files:
  dest=out/(kind+'-'+re.sub(r'[^\w.-]','_',name)+'-'+f.name+'.txt');shutil.copyfile(f,dest);links.append(f'[{f.name}](licenses/{dest.name})')
 rows.append([name,version,declared or 'See source notices',', '.join(links) or 'Missing: review required'])
for name,m in sorted(modules.items()):copy_licenses(name,m['Version'],m['Dir'],'go')
goroot=subprocess.check_output(['go','env','GOROOT'],text=True).strip();copy_licenses('Go runtime',subprocess.check_output(['go','version'],text=True).strip(),goroot,'go','BSD-3-Clause')
lock=json.loads(Path('apps/web/package-lock.json').read_text());runtime=set()
def visit(name):
 if name in runtime:return
 runtime.add(name);p=lock['packages']['node_modules/'+name]
 for dependency in p.get('dependencies',{}):visit(dependency)
for name in json.loads(Path('apps/web/package.json').read_text())['dependencies']:visit(name)
for name in sorted(runtime):
 p=lock['packages']['node_modules/'+name];copy_licenses(name,p['version'],'apps/web/node_modules/'+name,'npm',p.get('license',''))
lines=['# Third-party notices','','Runtime dependency notices collected from the locked Go and web dependency graphs. Texts are retained unmodified. Build/test dependencies and base-container packages are separately distributed by their upstream vendors; review their provenance when publishing images.','','The web runtime dependencies declare MIT or ISC licenses. The Go runtime/module notices use permissive BSD/MIT/ISC-style terms and SQLite public-domain attribution; preserve the supplied notices in redistributions. This inventory is a source-license review, not an independent legal opinion.','','| Dependency | Version | Declared / source license | Retained notice |','|---|---|---|---|']
lines+=['| '+' | '.join(row)+' |' for row in rows]
Path('docs/THIRD_PARTY_NOTICES.md').write_text('\n'.join(lines)+'\n')
print('Collected notices for',len(rows),'runtime dependencies')
