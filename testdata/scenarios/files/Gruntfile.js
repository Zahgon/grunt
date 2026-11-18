module.exports = function(grunt) {
  grunt.initConfig({
    name: 'bundle',
    files: {
      options: {sep: ';'},
      compact: {src: ['src/*.js', '!src/b.js'], dest: 'out/<%= name %>.js', extra: 1},
      object: {files: {'out/<%= name %>.min.js': ['src/**/*.js'], 'out/css': 'src/*.css'}},
      array: {files: [{src: 'src/sub/*', dest: 'out/sub/'}, {'out/x': [['src/a.js'], [['src/missing.js']]]}, {dest: 'out/nosrc'}]},
      expand: {expand: true, cwd: 'src/', src: ['**/*.js'], dest: 'out/exp/', ext: '.min.js', options: {flag: true}},
      flat: {expand: true, cwd: 'src', src: '**/*', dest: 'out/flat', flatten: true, filter: 'isFile', extDot: 'last', ext: '.x'},
      list: ['src/c.css', 'src/nope'],
      _hidden: {src: 'src/a.js'}
    }
  });
  grunt.registerMultiTask('files', 'Lists files.', function() {
    grunt.log.writeln('target ' + this.target + ' ' + JSON.stringify(this.options({sep: ',', z: 0})));
    this.files.forEach(function(f) {
      grunt.log.writeln(JSON.stringify({src: f.src, dest: f.dest, orig: f.orig}));
    });
    grunt.log.writeln('filesSrc ' + JSON.stringify(this.filesSrc));
  });
};
