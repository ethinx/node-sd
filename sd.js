(
  (
    _listen_port = os.env.SD_PORT || 9123,
    _default_node_port = os.env.SD_DEFAULT_NODE_PORT || "9100",
    _repo_addr = os.env.SD_REPO_ADDR || "127.0.0.1:6060",
    _interval = os.env.SD_INTERVAL || '5s',

    _repo_list = [],
    _sd_targets = [],

    repo_targets = (repo_info, _instances, _repo_targets, _target, _ip) => (
      _instances = repo_info?.['instances'] || {},
      _repo_targets = [],
      Object.keys(_instances).length > 0 ? (
        Object.entries(_instances).map(([_id, _property]) => (
          _ip = _property.ip.startsWith(':')? (
            _property.ip.replace('::ffff:', '')
          ): (
              _property.ip
            ),
          _target = _ip + ':' + _default_node_port,
          _repo_targets.includes(_target) || _repo_targets.push(_target)
        )),
        _repo_targets
      ) : _repo_targets
    ),
  )=>
  pipy({
    _repo: null,
  })

  // the API
  .listen(_listen_port)
  .serveHTTP(
    () => (
      new Message(
        {
          headers: {
            'content-type': 'application/json'
          },
        },
        JSON.encode(_sd_targets)
      )
    )
  )

  // the cron task
  .task(_interval)
  .onStart(()=>[new Message, new StreamEnd])
  .fork().to('list-repo')

  .pipeline('list-repo')
  .replaceMessage(
    ()=>new Message(
      {
        path: '/api/v1/repo',
        method: 'GET',
      }
    )
  )
  .muxHTTP().to(	
    $=>$
      .connect(()=>_repo_addr)
      .decodeHTTPResponse()
      .handleMessage(
        msg=>(
          _repo_list = msg.body.toString().split('\n')
          //console.log("repo list:", _repo_list)
        )
      ).link('get-repo')
  )

  .pipeline('get-repo')
  .onStart(()=>[new Message, new StreamEnd])
  .fork(()=>_repo_list).to(
    $=>$
      .onStart(repo=>void(_repo=repo, _sd_targets=[]))
      .replaceMessage(
        ()=>(
          new Message(
            {
              path: `/api/v1/repo${_repo}`,
              method: "GET",
            }
          )
        )
      )
      .muxHTTP().to('prom-targets')
  )

  .pipeline('prom-targets')
  .connect(()=>_repo_addr)
  .decodeHTTPResponse()
  .handleMessage(
    (msg, _resp, _repo_targets, _sd_target)=>(
      _resp = JSON.decode(msg.body),
      //console.log("response:", _resp['path']),
      _repo_targets = repo_targets(_resp),
      //console.log("targets:", _repo_targets),
      _resp && _sd_targets.push(
        {
          "targets": _repo_targets,
          "labels": {
            "repo": _resp['path']
          },
        }
      )
    )
  )
)()
