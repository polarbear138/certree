import{createRequire}from'node:module';
const require=createRequire(new URL('../apps/web/package.json',import.meta.url));
const parser=require('@apidevtools/swagger-parser');
const spec=await parser.validate(new URL('../docs/openapi.json',import.meta.url).pathname.replace(/^\/([A-Z]:\/)/,'$1'));
console.log('Validated OpenAPI',spec.openapi,'with',Object.keys(spec.paths).length,'paths');
