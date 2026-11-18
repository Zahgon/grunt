module.exports = function(grunt) {
  grunt.registerTask('warn', function() { grunt.log.writeln('before'); grunt.warn('careful', 9); grunt.log.writeln('after'); });
  grunt.registerTask('fatal', function() { grunt.fail.fatal(new Error('dead'), 7); });
  grunt.registerTask('errors', function() { grunt.log.error('one'); grunt.log.error(); grunt.log.writeln('count ' + this.errorCount); });
};
