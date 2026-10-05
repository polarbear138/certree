import{readFileSync,unlinkSync}from'node:fs';
import{resolve,dirname}from'node:path';
import{fileURLToPath}from'node:url';
export default function(){const file=resolve(dirname(fileURLToPath(import.meta.url)),'../.cache/e2e.pid');try{const pid=Number(readFileSync(file,'utf8'));if(Number.isInteger(pid)&&pid>0)process.kill(pid);unlinkSync(file)}catch{}}
