import json
import unittest
from model_client import sse_data


class FramingTest(unittest.TestCase):
    def test_utf8_continuation_bytes_are_not_newline_boundaries(self):
        original = {'choices':[{'delta':{'content':'腾出空间后，中文可以继续读取文件。'}}]}
        frame = json.dumps(original,ensure_ascii=False).encode()
        self.assertIn(0x85,frame)
        values = list(sse_data([b'data: '+frame,b'',b'data: [DONE]',b'']))
        self.assertEqual(json.loads(values[0]),original)
        self.assertEqual(values[1],'[DONE]')

    def test_multiline_events_and_comments(self):
        values = list(sse_data([b': ping',b'data: {"value":',b'data: 123}',b'']))
        self.assertEqual(json.loads(values[0]),{'value':123})


if __name__=='__main__':
    unittest.main()
