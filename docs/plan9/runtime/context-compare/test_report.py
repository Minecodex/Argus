import json
import unittest
from report import file_failure_counts


def tool(value):
    return {'role':'tool','content':json.dumps({'summary':value})}


class MetricsTest(unittest.TestCase):
    def test_one_bash_call_can_contain_several_missing_file_opens(self):
        value = file_failure_counts([tool({'exit_code':1,'stderr':
            "FileNotFoundError: first\nTraceback (most recent call last):\nFileNotFoundError: second\n"})])
        self.assertEqual(value,{'path_failure_tool_calls':1,'file_open_failures':2})

    def test_read_errors_and_other_script_failures_are_separate(self):
        value = file_failure_counts([tool({'error_code':'WORKSPACE_FILE_NOT_FOUND'}),
                                    tool({'exit_code':1,'stderr':"KeyError: 'values'\n"})])
        self.assertEqual(value,{'path_failure_tool_calls':1,'file_open_failures':1})

    def test_successfully_read_error_text_is_not_an_execution_failure(self):
        value = file_failure_counts([tool({'exit_code':0,'stdout':'FileNotFoundError: example',
                                          'stderr':'FileNotFoundError: illustrative text'})])
        self.assertEqual(value,{'path_failure_tool_calls':0,'file_open_failures':0})


if __name__=='__main__':
    unittest.main()
