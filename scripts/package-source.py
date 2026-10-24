"""Package reviewable source without installation secrets, generated keys, or caches."""
import os
from pathlib import Path
from zipfile import ZipFile, ZIP_DEFLATED
root=Path.cwd().resolve();out=root/'artifacts'/'certree-v0.1.0-candidate-source.zip';out.parent.mkdir(exist_ok=True)
excluded={'.git','.local','.cache','.agents','.codex','.aws','node_modules','dist','data','artifacts','test-results','playwright-report','__pycache__'}
count=0
with ZipFile(out,'w',ZIP_DEFLATED) as archive:
 for directory, dirs, names in os.walk(root):
  dirs[:]=sorted(d for d in dirs if d not in excluded)
  for name in sorted(names):
   file=Path(directory)/name
   if not file.is_file() or file.is_symlink():continue
   relative=file.relative_to(root)
   if file.name.startswith('coverage') or (file.name.startswith('.env') and file.name!='.env.example') or file.suffix in ['.exe','.db','.pem','.key','.pyc','.tsbuildinfo','.zip'] or '.db-' in file.name:continue
   archive.write(file,Path('certree')/relative);count+=1
print('Packaged',count,'source/doc files to',out)
