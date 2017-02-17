'use strict'

const {Command} = require('heroku-cli-command')
const path = require('path')
const dirs = require('../lib/dirs')
const currentChannel = require('../lib/channel')
const lock = require('rwlockfile')
const config = require('../lib/config')

class Update extends Command {
  async run () {
    this.fs = require('fs-extra')
    this.action('heroku-cli: Updating CLI')
    let channel = this.args.channel || currentChannel
    this.manifest = await this.fetchManifest(channel)
    if (!this.flags.force && config.version === this.manifest.version) {
      this.action.done(`already on latest version: ${config.version}`)
    } else if (!this.flags.force && config.updateDisabled) {
      this.action.done(`not updating CLI: ${config.updateDisabled}`)
    } else {
      this.action(`heroku-cli: Updating CLI to ${this.color.green(this.manifest.version)}${channel === 'stable' ? '' : ' (' + this.color.yellow(channel) + ')'}`)
      await this.update(channel)
      this.action.done()
    }
    this.action('heroku-cli: Updating plugins')
  }

  async fetchManifest (channel) {
    try {
      let url = `https://cli-assets.heroku.com/channels/${channel}/${process.platform}-${process.arch}`
      return await this.http.get(url)
    } catch (err) {
      if (err.statusCode === 403) throw new Error(`HTTP 403: Invalid channel ${channel}`)
      throw err
    }
  }

  async update (channel) {
    let url = `https://cli-assets.heroku.com/channels/${channel}/${this.base}.tar.gz`
    let stream = await this.http.get(url, {raw: true})
    let dir = path.join(dirs.data, 'jscli')
    let tmp = path.join(dirs.data, 'jscli_tmp')
    await this.extract(stream, tmp)
    await lock.write(dirs.updatelockfile, {skipOwnPid: true})
    this.fs.removeSync(dir)
    this.fs.renameSync(path.join(tmp, this.base), dir)
  }

  extract (stream, dir) {
    const zlib = require('zlib')
    const tar = require('tar-stream')

    return new Promise(resolve => {
      this.fs.removeSync(dir)
      let extract = tar.extract()
      extract.on('entry', (header, stream, next) => {
        let p = path.join(dir, header.name)
        let opts = {mode: header.mode}
        switch (header.type) {
          case 'directory':
            this.fs.mkdirpSync(p, opts)
            next()
            break
          case 'file':
            stream.pipe(this.fs.createWriteStream(p, opts))
            break
          case 'symlink':
            // ignore symlinks since they will not work on windows
            next()
            break
          default: throw new Error(header.type)
        }
        stream.resume()
        stream.on('end', next)
      })
      extract.on('finish', resolve)
      stream
      .pipe(zlib.createGunzip())
      .pipe(extract)
    })
  }

  get base () {
    return `heroku-v${this.manifest.version}-${process.platform}-${process.arch}`
  }

  async restartCLI () {
    await lock.read(dirs.updatelockfile)
    lock.unreadSync(dirs.updatelockfile)
    const {spawnSync} = require('child_process')
    const {status} = spawnSync('heroku', process.argv.slice(2), {stdio: 'inherit', shell: true})
    process.exit(status)
  }

  get autoupdateNeeded () {
    try {
      const fs = require('fs-extra')
      const moment = require('moment')
      const stat = fs.statSync(dirs.autoupdatefile)
      return moment(stat.mtime).isBefore(moment().subtract(4, 'hours'))
    } catch (err) {
      if (err.code !== 'ENOENT') console.error(err.stack)
      return true
    }
  }

  async autoupdate () {
    if (!this.autoupdateNeeded) return
    if (config.updateDisabled) return await this.warnIfUpdateAvailable()
    const fs = require('fs-extra')
    fs.writeFileSync(dirs.autoupdatefile, '')
    const {spawn} = require('child_process')
    spawn('heroku', ['update'])
  }

  async warnIfUpdateAvailable () {
    const manifest = await this.fetchManifest(currentChannel)
    if (version !== manifest.version) {
      console.error(`heroku-cli: update available from ${version} to ${manifest.version}`)
    }
  }

  /**
   * checks if there is an update running
   * if there is an update running, it waits for it to complete, then restarts the CLI
   * if no update running, checks if an update is needed and fires off an autoupdate process
   */
  async checkIfUpdating () {
    const lock = require('rwlockfile')
    if (await lock.hasWriter(dirs.updatelockfile)) {
      console.error('heroku-cli: warning: update in process')
      await this.restartCLI()
    } else {
      await lock.read(dirs.updatelockfile)
      await this.autoupdate()
    }
  }
}

Update.topic = 'update'
Update.args = [
  {name: 'channel', optional: true}
]
Update.flags = [
  {name: 'force', char: 'f', hidden: true}
]

module.exports = Update
