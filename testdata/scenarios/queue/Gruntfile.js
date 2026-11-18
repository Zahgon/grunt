module.exports = function(grunt) {
  grunt.registerTask('a', 'Task a.', function() {
    grunt.log.writeln('a ' + JSON.stringify(this.args) + ' ' + JSON.stringify(this.flags));
    grunt.task.run('b:1', 'c');
  });
  grunt.registerTask('b', function() {
    grunt.log.writeln('b ' + this.nameArgs);
    grunt.task.mark();
    grunt.task.run('c:x');
    grunt.task.clearQueue({untilMarker: true});
  });
  grunt.registerTask('c', function() { grunt.log.writeln('c ' + this.nameArgs); });
  grunt.registerTask('x:y', function() { grunt.log.writeln('x:y ' + JSON.stringify(this.args)); });
  grunt.registerTask('fails', function() { return false; });
  grunt.registerTask('throws', function() { throw new Error('boom'); });
  grunt.registerTask('async', function() {
    var done = this.async();
    setTimeout(function() { grunt.log.writeln('later'); done(false); }, 5);
  });
  grunt.registerTask('needs', function() {
    try { this.requires('c'); grunt.log.writeln('c ok'); } catch (e) { grunt.log.writeln(e.message); }
    this.requires('fails');
  });
  grunt.registerTask('stop', function() { grunt.task.clearQueue(); });
  grunt.registerTask('default', ['a', 'x:y:z']);
  grunt.registerTask('str', 'c');
  grunt.renameTask('missing', 'other');
};
