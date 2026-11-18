# Records the expected results of the scenario tests (scenario_test.go) with the
# original grunt 1.6.3 and grunt-cli 1.5.0 on Node 24:
#
#   docker run --rm -v $PWD/testdata/scenarios:/t -v <grunt 1.6.3 with node_modules>:/opt/original:ro \
#     node:24-bookworm python3 /t/record.py
#
# Each scenario directory holds a Gruntfile.js (the twin of the Go gruntfile in
# scenario_test.go), runs.json (the command lines) and the input files. The results
# go to expected.json: per scenario, each run's argv, exit code and stdout (with the
# scenario directory's path replaced by <dir>).
import json, os, subprocess, shutil, sys
out = {}
for sc in sorted(os.listdir('/t')):
    d = os.path.join('/t', sc)
    if not os.path.isdir(d):
        continue
    runs = json.load(open(os.path.join(d, 'runs.json')))
    res = []
    for argv in runs:
        w = '/tmp/w/' + sc
        shutil.rmtree('/tmp/w', ignore_errors=True)
        shutil.copytree(d, w)
        os.makedirs('/tmp/w/node_modules', exist_ok=True)
        os.symlink('/opt/original', '/tmp/w/node_modules/grunt')
        p = subprocess.run(['node', '/opt/original/bin/grunt'] + argv, cwd=w, capture_output=True, text=True,
                           env={'PATH': '/usr/local/bin:/usr/bin:/bin', 'HOME': '/tmp', 'TZ': 'UTC', 'LANG': 'C.UTF-8'})
        res.append({'argv': argv, 'code': p.returncode, 'out': p.stdout.replace(w, '<dir>')})
    out[sc] = res
json.dump(out, open('/t/expected.json', 'w'), indent=1, ensure_ascii=False)
