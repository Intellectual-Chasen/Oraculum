package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// openStages は、収集元の読み込みと処理の段階を組み、起動引数の収集元と調査に記録した収集元を
// 読み込む。
//
// **起動引数の収集元と記録した収集元は、待ち受けの前に同期で読み込む。** 読めない収集元の
// 起動を、今までどおり exit code で止める。読み込む収集元が無い起動は、要求による読み込みを
// 待つ。
//
// 調査の directory を渡さない起動は、分析者の値をメモリに置き、停止すると消える。渡した起動は、
// 調査を作るか開き、分析者の値を調査の SQLite file へ書く。そのとき返す調査は、呼び出し元が
// 閉じる。調査の directory を渡さない起動では nil を返す。
//
// layers は処理の段階でグラフを組む導出である。sigma は読み込みを終えた取り込み結果に Sigma の
// ルールを当てる。nil の起動は当てない。
func openStages(
	ctx context.Context, opts options, layers pipeline.GraphLayers, sigma *sigmaResult, stderr io.Writer,
) (*pipeline.Stages, *pipeline.Investigation, io.Closer, error) {
	config, err := importConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("collecting the declared input formats: %w", err)
	}
	startup, err := resolveStartup(ctx, opts, config.Parsers, stderr)
	if err != nil {
		return nil, nil, nil, err
	}
	if startup.open != nil {
		config.Open = startup.open
	}
	stages, closer, err := newStages(ctx, config, startup.recorder, startup.sourceRoot, layers, sigma, stderr)
	if err == nil && len(startup.plans) > 0 {
		err = stages.Load(startup.plans, startup.skipped)
	}
	if err != nil {
		var closeErr error
		if closer != nil {
			closeErr = closer.Close()
		}
		if startup.investigation != nil {
			closeErr = errors.Join(closeErr, startup.investigation.Close())
		}
		return nil, nil, nil, errors.Join(err, closeErr)
	}
	return stages, startup.investigation, closer, nil
}

// startup は、起動引数から決まる読み込みの記録先、基準の directory と、待ち受けの前に読む計画である。
type startup struct {
	recorder pipeline.ImportRecorder
	// investigation は開いた調査である。調査の directory を渡さない起動では nil である。
	investigation *pipeline.Investigation
	// sourceRoot は要求による読み込みの基準の directory である。空文字列の起動は要求による
	// 読み込みを受け付けない。
	sourceRoot string
	// open は待ち受けの前に読む計画を開く口である。nil のときは importConfig の口を使う。
	open  func(originPath string) (io.ReadCloser, error)
	plans []pipeline.SourcePlan
	// skipped は、起動引数の収集の directory にあり取り込まない file である。
	skipped []core.SkippedFile
}

// resolveStartup は、調査と基準の directory の起動引数から読み込みの組み方を決める。parsers は
// 収集の directory の file を署名で振り分けるパーサーの表である (pipeline.ExpandPlans)。
func resolveStartup(
	ctx context.Context, opts options, parsers map[core.FormatKey]pipeline.ParserFactory, stderr io.Writer,
) (startup, error) {
	sourceRoot := opts.sourceRoot
	if sourceRoot != "" {
		var err error
		if sourceRoot, err = filepath.Abs(sourceRoot); err != nil {
			return startup{}, fmt.Errorf("resolving the source root %q: %w", opts.sourceRoot, err)
		}
	}
	if opts.investigation == "" {
		// 起動引数の収集の directory は、基準の directory か起動した directory からの相対 path である。
		plans, skipped, err := pipeline.ExpandPlans(os.DirFS(cmp.Or(sourceRoot, ".")), opts.plans, parsers)
		if err != nil {
			return startup{}, fmt.Errorf("expanding the collection: %w", err)
		}
		resolved := startup{
			recorder: pipeline.NewMemoryRecorder(), sourceRoot: sourceRoot, plans: plans, skipped: skipped,
		}
		if sourceRoot != "" {
			// 起動引数の収集元も、基準の directory からの相対 path として読む。
			resolved.open = func(originPath string) (io.ReadCloser, error) {
				return os.Open(filepath.Join(sourceRoot, originPath)) // #nosec G304 G703 -- 運用者が起動引数で指定した原資料を開く。要求の path は openRequestedFiles が開く。
			}
		}
		return resolved, nil
	}
	investigation, added, skipped, err := prepareInvestigation(ctx, opts, sourceRoot, parsers)
	if err != nil {
		return startup{}, err
	}
	plans := append(investigation.RecordedPlans(), added...)
	if _, err := output.Fprintf(stderr, "investigation %s: %d recorded and %d added sources under %s\n",
		investigation.Dir(), len(plans)-len(added), len(added), investigation.SourceRoot()); err != nil {
		return startup{}, errors.Join(fmt.Errorf("writing the progress line: %w", err), investigation.Close())
	}
	return startup{
		recorder: investigation, investigation: investigation, sourceRoot: investigation.SourceRoot(),
		open: func(originPath string) (io.ReadCloser, error) {
			return os.Open(investigation.OriginFile(originPath)) // #nosec G304 G703 -- 調査に記録した、または運用者が起動引数で指定した原資料を開く。要求の path は openRequestedFiles が開く。
		},
		plans: plans, skipped: skipped,
	}, nil
}

// prepareInvestigation は調査を作るか開き、起動引数の収集の directory を展開して、起動引数の収集元が
// 記録と重ならないことを確かめる。展開した計画と、収集の directory の取り込まない file を返す。
func prepareInvestigation(
	ctx context.Context, opts options, sourceRoot string, parsers map[core.FormatKey]pipeline.ParserFactory,
) (*pipeline.Investigation, []pipeline.SourcePlan, []core.SkippedFile, error) {
	// 作成する調査は、--source-root が無ければ起動した directory を基準に記録する。収集元の
	// 引数がその directory からの相対 path であるためである。
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading the working directory: %w", err)
	}
	investigation, err := pipeline.PrepareInvestigation(ctx, opts.investigation, workingDir, sourceRoot)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("opening the investigation %q: %w", opts.investigation, err)
	}
	// 収集元も基準の directory も無い新しい調査は、読み込む手段が無い。
	if investigation.IsNew() && len(opts.plans) == 0 && opts.sourceRoot == "" {
		return nil, nil, nil, errors.Join(errors.New("creating the investigation: a source or "+sourceRootFlag+" is required"),
			investigation.Close())
	}
	// 収集の directory は、調査の基準の directory からの相対 path として辿る。
	plans, skipped, err := pipeline.ExpandPlans(os.DirFS(investigation.SourceRoot()), opts.plans, parsers)
	if err != nil {
		return nil, nil, nil, errors.Join(fmt.Errorf("expanding the collection: %w", err), investigation.Close())
	}
	if err := investigation.CheckAddedPlans(plans); err != nil {
		return nil, nil, nil, errors.Join(fmt.Errorf("adding sources to the investigation: %w", err),
			investigation.Close())
	}
	return investigation, plans, skipped, nil
}

// newStages は段階を組む。基準の directory がある起動は、要求による読み込みをその下だけで受け付ける。
func newStages(
	ctx context.Context, config pipeline.Config, recorder pipeline.ImportRecorder, sourceRoot string,
	layers pipeline.GraphLayers, sigma *sigmaResult, stderr io.Writer,
) (*pipeline.Stages, io.Closer, error) {
	var requested *pipeline.RequestedFiles
	var closer io.Closer
	if sourceRoot != "" {
		files, root, err := openRequestedFiles(sourceRoot)
		if err != nil {
			return nil, nil, err
		}
		requested, closer = files, root
	}
	stages, err := pipeline.NewStages(ctx, pipeline.StagesConfig{
		Import: config, Requested: requested, Recorder: recorder,
		Layers: api.WithCandidateOriginLog(layers),
		Clock:  pipeline.SystemClock{},
		Loaded: func(plans []pipeline.SourcePlan, result pipeline.ImportResult) error {
			if err := reportImport(stderr, plans, result); err != nil {
				return err
			}
			if sigma == nil {
				return nil
			}
			return sigma.evaluate(stderr, result)
		},
	})
	if err != nil {
		if closer != nil {
			err = errors.Join(err, closer.Close())
		}
		return nil, nil, fmt.Errorf("creating the stages: %w", err)
	}
	return stages, closer, nil
}
