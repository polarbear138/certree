"""Package reviewable source without installation secrets, generated keys, or caches."""
from pathlib import Path
from zipfile import ZipFile, ZIP_DEFLATED
root=Path.cwd().resolve();out=root/'artifacts'/'certree-v0.1.0-candidate-source.zip';out.parent.mkdir(exist_ok=True)
excluded={'.git','.cache','.agents','.codex','.aws','node_modules','dist','data','artifacts','test-results','playwright-report','__pycache__'}
count=0
with ZipFile(out,'w',ZIP_DEFLATED) as archive:
 for file in sorted(root.rglob('*')):
  if not file.is_file():continue
  relative=file.relative_to(root)
  if set(relative.parts)&excluded:continue
  if file.name.startswith('coverage') or (file.name.startswith('.env') and file.name!='.env.example') or file.suffix in ['.exe','.db','.pem','.key','.pyc','.tsbuildinfo']:continue
  archive.write(file,Path('certree')/relative);count+=1
print('Packaged',count,'source/doc files to',out)
