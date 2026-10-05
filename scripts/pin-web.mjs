import{readFileSync,writeFileSync}from'node:fs';
const file='apps/web/package.json';const p=JSON.parse(readFileSync(file));const lock=JSON.parse(readFileSync('apps/web/package-lock.json'));
for(const section of ['dependencies','devDependencies'])for(const name of Object.keys(p[section]))p[section][name]=lock.packages['node_modules/'+name].version;
writeFileSync(file,JSON.stringify(p,null,2)+'\n');
