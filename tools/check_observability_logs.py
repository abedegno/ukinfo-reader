#!/usr/bin/env python3
"""Exercise the deployed Collector log privacy policy with invented OTLP/HEC data."""
import json,subprocess,tempfile,uuid
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
IMAGE='otel/opentelemetry-collector-contrib:0.163.1@sha256:1175b03003053962012f5a231f5567f66a3e8fdb960c7835c0e48f7aa1555806'
FIXTURE=r'''
import gzip,http.server,json,threading,time,urllib.request
received=[]
class HEC(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_POST(self):
  assert self.headers.get('Authorization')=='Splunk invented-hec-token'
  raw=self.rfile.read(int(self.headers['Content-Length']))
  if self.headers.get('Content-Encoding')=='gzip':raw=gzip.decompress(raw)
  assert b'SECRET' not in raw,'Privacy canary escaped'
  decoder=json.JSONDecoder();text=raw.decode()
  while text.strip():
   obj,end=decoder.raw_decode(text.lstrip());text=text.lstrip()[end:];received.append(obj)
  self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(b'{"text":"Success","code":0}')
threading.Thread(target=http.server.HTTPServer(('127.0.0.1',18088),HEC).serve_forever,daemon=True).start()
def kv(k,v):return {'key':k,'value':{'intValue':str(v)} if isinstance(v,int) else {'stringValue':v}}
def record(poison=False):
 attrs=[kv('event.name','ukinfo.http.request'),kv('http.route','SECRET_ROUTE' if poison else '/api/search'),kv('http.request.method','SECRET_METHOD' if poison else 'GET'),kv('http.response.status_code',503),kv('duration_ms',120),kv('ukinfo.query.class','SECRET_CLASS' if poison else 'name'),kv('ukinfo.failure.reason','SECRET_ERROR' if poison else 'query_timeout'),kv('request_id','SECRET_REQUEST_ID' if poison else '0123456789abcdef01234567'),kv('url.full','SECRET_QUERY'),kv('com.splunk.index','SECRET_INDEX')]
 return {'timeUnixNano':str(time.time_ns()),'body':{'stringValue':'HTTP request completed'},'attributes':attrs,'traceId':'11111111111111111111111111111111','spanId':'2222222222222222','severityText':'SECRET_SEVERITY'}
raw=record();raw['body']={'stringValue':'SECRET_RAW_NAME_ADDRESS'}
payload={'resourceLogs':[{'resource':{'attributes':[kv('service.name','SECRET_SERVICE'),kv('private','SECRET_RESOURCE')]},'scopeLogs':[{'scope':{'name':'SECRET_SCOPE','version':'SECRET_VERSION','attributes':[kv('private','SECRET_SCOPE_ATTR')]},'logRecords':[record(),record(True),raw]}]}]}
for _ in range(50):
 try:
  urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:4318/v1/logs',data=json.dumps(payload).encode(),headers={'Content-Type':'application/json'}),timeout=2).read();break
 except OSError:time.sleep(.2)
else:raise AssertionError('Collector did not accept OTLP logs')
for _ in range(100):
 if len(received)>=2:break
 time.sleep(.1)
assert len(received)==2,'Expected two sanitized events and one dropped raw event'
for item in received:
 assert item['index']=='ukinfo_dev' and item['sourcetype']=='otel'
wire=json.dumps(received)
assert '11111111111111111111111111111111' in wire and '2222222222222222' in wire,'Trace correlation lost'
assert 'ERROR' in wire,'Severity not normalized'
open('/out/receipt.json','w').write(json.dumps({'safe_events':len(received),'raw_event_dropped':True,'privacy_canaries_absent':True,'trace_context_preserved':True}))
'''

def run(*args,**kwargs):return subprocess.run(args,check=True,capture_output=True,text=True,**kwargs)
def main():
 name='ukinfo-log-check-'+uuid.uuid4().hex[:10]
 with tempfile.TemporaryDirectory(prefix='ukinfo-log-check-') as tmp:
  root=Path(tmp);root.chmod(0o755);out=root/'out';out.mkdir(mode=0o777);out.chmod(0o777)
  config=(ROOT/'deploy/observability/collector.logs.yaml').read_text().replace('${env:SPLUNK_HEC_ENDPOINT}','http://127.0.0.1:18088/services/collector').replace('    tls:\n      ca_file: /etc/otelcol/splunk-ca.pem\n','')
  (root/'collector.yaml').write_text(config);(root/'hec-token').write_text('invented-hec-token');(root/'ingest-token').write_text('invented-ingest-token')
  try:
   run('docker','run','-d','--name',name,'--network','none','--read-only','--cap-drop=ALL','--security-opt=no-new-privileges','--memory=128m','--cpus=0.25','--env','SPLUNK_REALM=eu0','--volume',str(root/'collector.yaml')+':/etc/otelcol/config.yaml:ro','--volume',str(root/'hec-token')+':/run/secrets/splunk_hec_token:ro','--volume',str(root/'ingest-token')+':/run/secrets/splunk_ingest_token:ro',IMAGE,'--config=/etc/otelcol/config.yaml')
   run('docker','run','--rm','--network','container:'+name,'--volume',str(out)+':/out','python:3.13-slim','python','-c',FIXTURE,timeout=40)
   print((out/'receipt.json').read_text())
  except subprocess.CalledProcessError as error:
   print(error.stderr[-2500:]);print(run('docker','logs',name).stderr[-3500:]);raise
  finally:subprocess.run(['docker','rm','-f',name],capture_output=True)
if __name__=='__main__':main()
