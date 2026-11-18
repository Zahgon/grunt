module.exports = function(grunt) {
  grunt.initConfig({
    pkg: {name: 'demo', list: [1, 2]},
    paths: {src: 'src/<%= pkg.name %>', dist: '<%= paths.src %>/dist', all: '<%= pkg.list %>', upper: '<%= pkg.name.toUpperCase() %>'},
    'a.b': {c: 'dotted'},
    cyc: '<%= cyc %>',
    bad: '<%= nope %>'
  });
  grunt.registerTask('cfg', function() {
    grunt.log.writeln(JSON.stringify(grunt.config.get('paths.dist')));
    grunt.log.writeln(JSON.stringify(grunt.config.get('paths.all')));
    grunt.log.writeln(JSON.stringify(grunt.config.get('paths.upper')));
    grunt.log.writeln(JSON.stringify(grunt.config.get(['a.b', 'c'])));
    grunt.log.writeln(JSON.stringify(grunt.config.get('a\\.b.c')));
    grunt.log.writeln(JSON.stringify(grunt.config.getRaw('paths.src')));
    grunt.log.writeln(JSON.stringify(grunt.config.process({k: ['<%= pkg.name %>-x', 3]})));
    grunt.log.writeln(grunt.config.escape('a.b.c') + ' ' + grunt.config.getPropString(['a.b', 'c']));
    grunt.config.set('new.deep', '<%= pkg.name %>!');
    grunt.log.writeln(JSON.stringify(grunt.config('new')));
    try { grunt.config.get('cyc'); } catch (e) { grunt.log.writeln('caught: ' + e.message); }
    try { grunt.config.requires('pkg.name', ['missing', 'x']); } catch (e) { grunt.log.writeln('caught: ' + e.message); }
    grunt.log.writeln(JSON.stringify(grunt.config.merge({pkg: {list: ['m']}}).pkg));
  });
  grunt.registerTask('bad', function() {
    grunt.log.writeln('got ' + JSON.stringify(grunt.config.get('bad')));
  });
};
