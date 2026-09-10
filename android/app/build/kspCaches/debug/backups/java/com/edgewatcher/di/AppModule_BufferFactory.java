package com.edgewatcher.di;

import android.content.Context;
import com.edgewatcher.domain.port.ObservationBuffer;
import com.edgewatcher.infrastructure.db.ObservationDatabase;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata("dagger.hilt.android.qualifiers.ApplicationContext")
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_BufferFactory implements Factory<ObservationBuffer> {
  private final Provider<Context> contextProvider;

  private final Provider<ObservationDatabase> dbProvider;

  private AppModule_BufferFactory(Provider<Context> contextProvider,
      Provider<ObservationDatabase> dbProvider) {
    this.contextProvider = contextProvider;
    this.dbProvider = dbProvider;
  }

  @Override
  public ObservationBuffer get() {
    return buffer(contextProvider.get(), dbProvider.get());
  }

  public static AppModule_BufferFactory create(Provider<Context> contextProvider,
      Provider<ObservationDatabase> dbProvider) {
    return new AppModule_BufferFactory(contextProvider, dbProvider);
  }

  public static ObservationBuffer buffer(Context context, ObservationDatabase db) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.buffer(context, db));
  }
}
