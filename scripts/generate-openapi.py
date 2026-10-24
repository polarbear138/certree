"""Regenerate the API contract. Keep endpoint schemas aligned with handlers."""
import json
from pathlib import Path
string={"type":"string"}
schemas={
 "Error":{"type":"object","required":["error"],"properties":{"error":string}},
 "Identity":{"type":"object","properties":{"User":string,"CSRF":string}},
 "ID":{"type":"object","properties":{"id":string}},
 "Login":{"type":"object","additionalProperties":False,"required":["username","password"],"properties":{"username":string,"password":{"type":"string","writeOnly":True}}},
 "Password":{"type":"object","additionalProperties":False,"required":["current_password","new_password"],"properties":{"current_password":{"type":"string","writeOnly":True},"new_password":{"type":"string","minLength":14,"maxLength":72,"writeOnly":True}}},
 "Organization":{"type":"object","additionalProperties":False,"required":["name"],"properties":{"name":{"type":"string","minLength":1,"maxLength":120}}},
 "Environment":{"type":"object","additionalProperties":False,"required":["name","organization_id"],"properties":{"name":string,"organization_id":string}},
 "Server":{"type":"object","additionalProperties":False,"required":["name","host","port","user","type","host_key","ssh_key"],"properties":{"name":string,"host":string,"port":{"type":"integer","minimum":1,"maximum":65535},"user":string,"type":{"const":"nginx"},"host_key":{"type":"string","description":"Independently verified pinned OpenSSH host public key"},"ssh_key":{"type":"string","writeOnly":True,"description":"Unencrypted input PEM/OpenSSH key, encrypted at rest"}}},
 "Target":{"type":"object","additionalProperties":False,"required":["server_id","certificate_id","host","port","sni","certificate_path","key_path","configuration_path","backup_path"],"properties":{k:string for k in ["server_id","endpoint_id","certificate_id","host","sni","certificate_path","key_path","configuration_path","backup_path"]}},
 "JobRequest":{"type":"object","additionalProperties":False,"required":["target_id","version_id"],"properties":{"target_id":string,"version_id":string,"kind":{"type":"string","enum":["Deployment","Verification","Recovery"],"default":"Deployment","description":"Recovery deploys an explicitly selected previous approved valid version"}}},
 "BulkPreview":{"type":"object","additionalProperties":False,"required":["target_ids"],"properties":{"target_ids":{"type":"array","minItems":1,"maxItems":200,"uniqueItems":True,"items":string}}},
 "BulkRequest":{"type":"object","additionalProperties":False,"required":["items"],"properties":{"items":{"type":"array","minItems":1,"maxItems":200,"items":{"type":"object","additionalProperties":False,"required":["target_id","version_id"],"properties":{"target_id":string,"version_id":string}}}}},
 "Settings":{"type":"object","additionalProperties":False,"required":["expiry_days"],"properties":{"expiry_days":{"type":"integer","minimum":1,"maximum":365}}},
 "Collector":{"type":"object","additionalProperties":False,"required":["name","environment_id","host","port","username","password","interval"],"properties":{k:string for k in ["name","environment_id","host","username","mailbox","from","subject"]}},
 "Upload":{"type":"object","required":["environment_id","files"],"properties":{"environment_id":string,"password":{"type":"string","writeOnly":True,"description":"PKCS#12 import password; not retained"},"files":{"type":"array","minItems":1,"maxItems":64,"items":{"type":"string","format":"binary"},"description":"16 MiB aggregate input maximum; content detected"}}},
 "Node":{"type":"object","required":["id","entity_id","type","name","status","counts","children"],"properties":{"id":string,"entity_id":string,"type":{"enum":["root","organization","environment","certificate","target"]},"name":string,"status":{"enum":["Healthy","Warning","Failed","Pending","Deploying","Unknown"]},"counts":{"type":"object","additionalProperties":{"type":"integer"}},"children":{"type":"array","items":{"$ref":"#/components/schemas/Node"}},"data":{"type":"object","description":"Public entity metadata, never secret material"}}},
 "Validation":{"type":"object","properties":{"valid":{"type":"boolean"},"key_match":{"type":"boolean"},"trusted":{"type":"boolean"},"errors":{"type":"array","items":string}}},
 "List":{"type":"array","items":{"type":"object"}},
 "Detail":{"type":"object"},
}
schemas['Target']['properties'].update(port={"type":"integer","minimum":1,"maximum":65535},sudo={"type":"boolean","default":False})
schemas['Login']['properties'].update(username={"type":"string","minLength":1,"maxLength":120},password={"type":"string","maxLength":72,"writeOnly":True})
schemas['Collector']['properties'].update(port={"type":"integer","minimum":1,"maximum":65535},password={"type":"string","writeOnly":True},interval={"type":"integer","minimum":60,"maximum":86400})
spec={"openapi":"3.1.0","info":{"title":"Certree API","version":"0.1.0","description":"Administrator-only API. JSON metadata fields in SQL lists are serialized JSON strings; tree data is expanded. Uploaded trust roots do not grant issuer trust. All state changes require matching Origin and session CSRF header."},"servers":[{"url":"/api"}],"security":[{"session":[]}],"paths":{},"components":{"securitySchemes":{"session":{"type":"apiKey","in":"cookie","name":"certree_session"}},"schemas":schemas}}
def op(path,method,summary,response='Detail',body=None,status=200,public=False):
 o={"summary":summary,"operationId":method+'_'+path.replace('/','_').replace('{','').replace('}',''),"responses":{str(status):{"description":"Successful request","content":{"application/json":{"schema":{"$ref":"#/components/schemas/"+response}}}},"default":{"description":"Rejected request; 400 invalid input, 401 session required, 403 CSRF/Origin denied, 404 absent entity, 409 active job/conflict, 422 failed validation, 429 login limit, 500 internal error","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Error"}}}}}}
 params=[]
 if '{id}' in path:params.append({"in":"path","name":"id","required":True,"schema":string})
 if method not in ['get','head']:
  params.append({"in":"header","name":"Origin","required":True,"schema":string,"description":"Must equal CERTREE_ORIGIN"})
  if not public:params.append({"in":"header","name":"X-CSRF-Token","required":True,"schema":string})
 if params:o['parameters']=params
 if public:o['security']=[]
 if body:o['requestBody']={"required":True,"content":{'multipart/form-data' if body=='Upload' else 'application/json':{"schema":{"$ref":"#/components/schemas/"+body}}}}
 spec['paths'].setdefault(path,{})[method]=o
op('/system/health','get','Database availability and version',public=True)
op('/auth/login','post','Create an eight-hour browser session','Identity','Login',public=True)
op('/auth/me','get','Current administrator and session CSRF token','Identity')
op('/auth/logout','post','Revoke current session')
op('/auth/password','post','Rotate password and revoke every session',body='Password')
op('/tree','get','Relational tree projection','Node')
spec['paths']['/tree']['get']['parameters']=[{"in":"query","name":k,"schema":string} for k in ['q','status']]
op('/tree/summary','get','Inventory counts and aggregated status')
op('/tree/nodes/{id}','get','Stable tree node details','Node')
for name,schema in [('organizations','Organization'),('environments','Environment'),('servers','Server'),('targets','Target'),('collectors','Collector')]:
 op('/'+name,'get','List '+name,'List')
 op('/'+name,'post','Create '+name,'ID',schema,status=201)
op('/certificates','get','Latest versions and public validation metadata','List')
op('/certificates','post','Import bundle; returns version_id','Detail','Upload',status=201)
op('/certificates/{id}','get','Certificate, immutable versions, and targets')
op('/certificates/versions/{id}/approve','post','Approve a currently trusted valid version','Validation')
op('/servers/{id}','get','Public registered server details')
op('/servers/{id}/test','post','Pinned SSH authentication and SFTP check')
op('/deployments','get','Latest 200 jobs filtered by server and kind','List')
spec['paths']['/deployments']['get']['parameters']=[{'in':'query','name':'server_id','schema':string},{'in':'query','name':'kind','schema':{'type':'string','enum':['Deployment','Verification']}}]
op('/deployments','post','Explicitly approve and queue target operation','ID','JobRequest',status=202)
op('/deployments/bulk/preview','post','Review up to 200 targets using explicit approved certificate versions',body='BulkPreview')
op('/deployments/bulk','post','Queue reviewed versions with per-target results and server serialization',body='BulkRequest')
op('/deployments/{id}','get','Job, stage events, and TLS evidence')
op('/deployments/{id}/cancel','post','Cancel a Queued or Pending job only')
op('/collectors/{id}/poll','post','Queue a poll for the next worker polling cycle',status=202)
op('/collectors/{id}/test','post','Test verified IMAP TLS login and mailbox access')
for name in ['collector-events','verifications','audit-logs']:op('/'+name,'get','Latest 200 public '+name,'List')
op('/settings','get','Current expiry warning configuration','List')
op('/settings','put','Set expiry warning days',body='Settings')
schemas.update({
 'AgentEnrollment': {'type':'object','additionalProperties':False,'required':['environment_id'],'properties':{'environment_id':string}},
 'AgentRegistration': {'type':'object','additionalProperties':False,'required':['token','name'],'properties':{'token':{'type':'string','minLength':64,'maxLength':64,'writeOnly':True},'name':{'type':'string','minLength':1,'maxLength':120}}},
 'AgentCredentials': {'type':'object','required':['id','token'],'properties':{'id':string,'token':{'type':'string','description':'Secret returned once; store locally with restricted permissions'}}},
 'AgentTarget': {'type':'object','additionalProperties':False,'required':['id','certificate_id','type','certificate_path','key_path','configuration_path','host','port','sni'],'properties':{k:string for k in ['id','certificate_id','type','certificate_path','key_path','configuration_path','host','sni']}},
 'AgentResult': {'type':'object','additionalProperties':False,'required':['phase','outcome'],'properties':{'phase':{'enum':['deploy','rollback']},'outcome':{'enum':['Reloaded','Rolled Back','Failed','Recovery Failed']},'fingerprint':string,'error':{'type':'string','maxLength':2048}}},
 'AgentTask': {'type':'object','properties':{k:string for k in ['id','target_id','phase','fingerprint','certificate_pem','key_pem']}},
 'AgentPoll': {'type':'object','required':['task'],'properties':{'task':{'oneOf':[{'type':'null'},{'$ref':'#/components/schemas/AgentTask'}]}}},
})
schemas['AgentTarget']['properties'].update(type={'const':'nginx'},port={'type':'integer','minimum':1,'maximum':65535})
spec['info']['version']='0.2.0-dev'
spec['info']['description']='Browser administrator API uses sessions, Origin and CSRF. Agent v1 uses per-agent bearer credentials over HTTPS; enrollment uses a single-use 15-minute key scoped to an environment. Agent reload reports never imply successful deployment until central live TLS verification.'
spec['components']['securitySchemes']['agent']={'type':'http','scheme':'bearer'}
op('/agents','get','List agents without credentials','List')
op('/agents/enrollment','post','Create a single-use environment-scoped registration key',body='AgentEnrollment',status=201)
op('/agents/{id}/revoke','post','Reject subsequent agent requests; in-flight local work may still complete')
for path,method,summary,response,body,status in [
 ('/agent/v1/register','post','Consume registration key and return agent credentials','AgentCredentials','AgentRegistration',201),
 ('/agent/v1/inventory','get','Public certificate choices in assigned environment','List',None,200),
 ('/agent/v1/targets','post','Register immutable locally configured target; repeated identical requests return 200','ID','AgentTarget',201),
 ('/agent/v1/poll','post','Heartbeat and fetch current deploy or rollback task; repeats until result is durable','AgentPoll',None,200),
 ('/agent/v1/jobs/{id}/result','post','Persist idempotent result for central verification','Detail','AgentResult',200),
 ('/agent/v1/history','get','Latest 100 jobs for this agent','List',None,200),
]:
 op(path,method,summary,response,body,status,public=True)
 entry=spec['paths'][path][method]
 entry['parameters']=[p for p in entry.get('parameters',[]) if p['in']!='header']
 entry['security']=[] if path.endswith('/register') else [{'agent':[]}]
spec['paths']['/agent/v1/targets']['post']['responses']['200']=spec['paths']['/agent/v1/targets']['post']['responses']['201']
Path('docs').mkdir(exist_ok=True)
Path('docs/openapi.json').write_text(json.dumps(spec,indent=2)+'\n')
